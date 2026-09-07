package engineering

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/config"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/jobs"
	"github.com/hurtener/chartworks/internal/sources"
	"github.com/jackc/pgx/v5"
)

func (s *PipelineService) connection(e identity.Envelope, alias string) (config.SourceConnection, error) {
	for _, c := range s.values.Sources.Connections {
		if c.Tenant == e.Tenant() && c.ID == alias && c.ManagedSchema != "" {
			return c, nil
		}
	}
	return config.SourceConnection{}, ErrOwnership
}

// pipelineTx uses a separate restricted writer connection; no read adapter can
// obtain it. The workspace marker binds the physical schema to this tenant/alias.
func (s *PipelineService) pipelineTx(ctx context.Context, c config.SourceConnection, fn func(pgx.Tx, *pgx.ConnConfig, *pgx.ConnConfig) error) error {
	w, r, err := managedWriterConfig(s.values, s.lookup, s.repo.DatabaseName(), c)
	if err != nil {
		return err
	}
	if err = sources.RequirePipelineReadLocation(ctx, w); err != nil {
		return err
	}
	conn, err := pgx.ConnectConfig(ctx, w)
	if err != nil {
		return ErrUnavailable
	}
	defer func() {
		clean, stop := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
		defer stop()
		_ = conn.Close(clean)
	}()
	tx, err := conn.Begin(ctx)
	if err != nil {
		return ErrUnavailable
	}
	defer func() {
		clean, stop := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
		defer stop()
		_ = tx.Rollback(clean)
	}()
	var oid int64
	var valid bool
	err = tx.QueryRow(ctx, `SELECT r.oid::bigint,current_user=session_user AND current_user=$1 AND current_database()=$2 AND NOT(rolsuper OR rolbypassrls OR rolcreaterole OR rolcreatedb) AND current_setting('server_version_num')::integer BETWEEN 170000 AND 179999 FROM pg_catalog.pg_roles r WHERE rolname=current_user`, w.User, w.Database).Scan(&oid, &valid)
	if err != nil {
		return ErrUnavailable
	}
	if !valid {
		return ErrOwnership
	}
	schema, _, err := sources.ManagedLocation(c, "pipeline")
	if err != nil {
		return ErrOwnership
	}
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,7214061111))`, c.Tenant+":"+c.ID); err != nil {
		return err
	}
	if err = ensureWorkspace(ctx, tx, c, schema, oid, w.User, r.User); err != nil {
		return err
	}
	registry := pgx.Identifier{schema, "_pipeline_stages"}.Sanitize()
	var exists bool
	if err = tx.QueryRow(ctx, `SELECT to_regclass($1) IS NOT NULL`, registry).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		if _, err = tx.Exec(ctx, "CREATE TABLE "+registry+" (operation_id text NOT NULL, step_id text NOT NULL, manifest_hash text NOT NULL, table_name text NOT NULL UNIQUE, table_oid bigint, PRIMARY KEY(operation_id,step_id))"); err != nil {
			return err
		}
	}
	if _, err = ownedTable(ctx, tx, schema, "_pipeline_stages", 0); err != nil {
		return ErrOwnership
	}
	if err = fn(tx, w, r); err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	if err = tx.Commit(ctx); err != nil {
		return ErrPipelineUncertain
	}
	return nil
}

func pipelineInputsWritable(ctx context.Context, tx pgx.Tx, b readexec.Binding, stage sources.PipelineStage, dependencies []string) error {
	wanted := map[string]bool{}
	for _, id := range dependencies {
		wanted[id] = true
	}
	for _, r := range b.Relations {
		if !wanted[r.ID] {
			continue
		}
		relation := pgx.Identifier{r.Schema, r.Name}.Sanitize()
		var safe bool
		if err := tx.QueryRow(ctx, `SELECT has_table_privilege(current_user,$1,'SELECT') AND NOT has_table_privilege(current_user,$1,'INSERT,UPDATE,DELETE,TRUNCATE,REFERENCES,TRIGGER')`, relation).Scan(&safe); err != nil {
			return ErrOwnership
		}
		if !safe {
			// Same-run predecessor writes are owned by this runner, but their read
			// callback already holds exact OID locks and verified signed stage reach.
			registry := pgx.Identifier{stage.Schema, "_pipeline_stages"}.Sanitize()
			if r.Schema != stage.Schema {
				return ErrOwnership
			}
			if err := tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM "+registry+" WHERE operation_id=$1 AND table_name=$2 AND table_oid=to_regclass($3)::oid::bigint) OR EXISTS(SELECT 1 FROM "+pgx.Identifier{stage.Schema, "_uploads"}.Sanitize()+" WHERE table_name=$2 AND table_oid=to_regclass($3)::oid::bigint AND state='loaded')", stage.Operation, r.Name, relation).Scan(&safe); err != nil || !safe {
				return ErrOwnership
			}
		}
	}
	return nil
}

func (s *PipelineService) preparePipelineStage(ctx context.Context, c config.SourceConnection, record PipelineRecord, state PipelineStageState, b readexec.Binding, dependencies []string) (bool, bool, error) {
	compensated := false
	fresh := state.Previous == nil || pipelineStep(record.Definition, state.Stage.Step).Strategy == "replace"
	err := s.pipelineTx(ctx, c, func(tx pgx.Tx, w, r *pgx.ConnConfig) error {
		if err := pipelineInputsWritable(ctx, tx, b, state.Stage, dependencies); err != nil {
			return err
		}
		stage := state.Stage
		registry := pgx.Identifier{stage.Schema, "_pipeline_stages"}.Sanitize()
		target := pgx.Identifier{stage.Schema, stage.Table}.Sanitize()
		var hash, name string
		var recorded *int64
		err := tx.QueryRow(ctx, "SELECT manifest_hash,table_name,table_oid FROM "+registry+" WHERE operation_id=$1 AND step_id=$2 FOR UPDATE", stage.Operation, stage.Step).Scan(&hash, &name, &recorded)
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			var exists bool
			if err = tx.QueryRow(ctx, `SELECT to_regclass($1) IS NOT NULL`, target).Scan(&exists); err != nil {
				return err
			}
			if exists {
				return ErrOwnership
			}
			if _, err = tx.Exec(ctx, "INSERT INTO "+registry+" (operation_id,step_id,manifest_hash,table_name) VALUES($1,$2,$3,$4)", stage.Operation, stage.Step, record.Digest, stage.Table); err != nil {
				return err
			}
		case err != nil:
			return err
		default:
			if hash != record.Digest || name != stage.Table {
				return ErrOwnership
			}
			// A prior dispatched attempt must first be known physically stopped. Never
			// cancel a reusable backend PID or retry an unknown live side effect.
			applications := append(append([]string(nil), state.PriorApplications...), state.Application)
			for _, application := range applications {
				if application == "" {
					continue
				}
				var active bool
				if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_catalog.pg_stat_activity WHERE datname=current_database() AND usename=current_user AND application_name=$1 AND pid<>pg_backend_pid())`, application).Scan(&active); err != nil {
					return err
				}
				if active {
					return ErrPipelineUncertain
				}
			}
			var current *int64
			if err = tx.QueryRow(ctx, `SELECT to_regclass($1)::oid::bigint`, target).Scan(&current); err != nil {
				return err
			}
			if current != nil {
				// Only the registry's exact OID permits compensation. An unrecorded table
				// after a lost runner response remains uncertain instead of guessed owned.
				if recorded == nil || *recorded != *current {
					return ErrPipelineUncertain
				}
				if _, err = ownedTable(ctx, tx, stage.Schema, stage.Table, *recorded); err != nil {
					return ErrOwnership
				}
				if _, err = tx.Exec(ctx, "DROP TABLE "+target); err != nil {
					return err
				}
				compensated = true
			}
		}
		if !fresh {
			prev := state.Previous
			if prev == nil || !prev.Valid() {
				return ErrOwnership
			}
			if _, err = ownedPipelineReadTable(ctx, tx, prev.Schema, prev.Table, prev.OID); err != nil {
				return ErrOwnership
			}
			previous := pgx.Identifier{prev.Schema, prev.Table}.Sanitize()
			if _, err = tx.Exec(ctx, "LOCK TABLE "+previous+" IN ACCESS SHARE MODE"); err != nil {
				return err
			}
			if _, err = tx.Exec(ctx, "CREATE TABLE "+target+" (LIKE "+previous+" INCLUDING ALL)"); err != nil {
				return err
			}
			if _, err = tx.Exec(ctx, "INSERT INTO "+target+" SELECT * FROM "+previous); err != nil {
				return err
			}
			var oid int64
			if err = tx.QueryRow(ctx, `SELECT to_regclass($1)::oid::bigint`, target).Scan(&oid); err != nil {
				return err
			}
			if _, err = tx.Exec(ctx, "UPDATE "+registry+" SET table_oid=$3 WHERE operation_id=$1 AND step_id=$2", stage.Operation, stage.Step, oid); err != nil {
				return err
			}
		} else {
			if _, err = tx.Exec(ctx, "UPDATE "+registry+" SET table_oid=NULL WHERE operation_id=$1 AND step_id=$2", stage.Operation, stage.Step); err != nil {
				return err
			}
		}
		return nil
	})
	return fresh, compensated, err
}

func pipelineStep(d PipelineDefinition, id string) PipelineStep {
	for _, s := range d.Steps {
		if s.ID == id {
			return s
		}
	}
	return PipelineStep{}
}

func (s *PipelineService) inspectPipelineStage(ctx context.Context, c config.SourceConnection, step PipelineStep, state PipelineStageState) (PipelineStageState, error) {
	quality := true
	err := s.pipelineTx(ctx, c, func(tx pgx.Tx, w, r *pgx.ConnConfig) error {
		stage := &state.Stage
		target := pgx.Identifier{stage.Schema, stage.Table}.Sanitize()
		oid, err := ownedTable(ctx, tx, stage.Schema, stage.Table, 0)
		if err != nil {
			return ErrOwnership
		}
		if _, err = tx.Exec(ctx, "LOCK TABLE "+target+" IN ACCESS EXCLUSIVE MODE"); err != nil {
			return err
		}
		stage.OID = oid
		rows, err := tx.Query(ctx, `SELECT attname,atttypid::bigint FROM pg_catalog.pg_attribute WHERE attrelid=$1::oid AND attnum>0 AND NOT attisdropped ORDER BY attnum`, oid)
		if err != nil {
			return err
		}
		columns := []string{}
		types := map[string]int64{}
		for rows.Next() {
			var name string
			var typeID int64
			if err = rows.Scan(&name, &typeID); err != nil {
				rows.Close()
				return err
			}
			columns = append(columns, name)
			types[name] = typeID
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		if readexec.Hash(columns) != readexec.Hash(stage.Columns) {
			quality = false
		}
		expectedTypes := map[string]int64{"bigint": 20, "numeric": 1700, "text": 25, "boolean": 16, "date": 1082, "timestamp": 1114, "timestamptz": 1184, "bytea": 17}
		for _, column := range step.Columns {
			if types[column.Name] != expectedTypes[column.Type] {
				quality = false
			}
		}
		if step.Strategy == "scd2" && (types["_valid_from"] != 1184 || types["_valid_until"] != 1184 || types["_is_current"] != 16) {
			quality = false
		}
		if err = tx.QueryRow(ctx, "SELECT count(*) FROM "+target).Scan(&state.Rows); err != nil {
			return err
		}
		keys := []string{}
		for _, column := range step.Columns {
			if column.PrimaryKey && (step.Strategy == "merge" || step.Strategy == "scd2") {
				name := pgx.Identifier{column.Name}.Sanitize()
				keys = append(keys, name)
				var valid bool
				if err = tx.QueryRow(ctx, "SELECT NOT EXISTS(SELECT 1 FROM "+target+" WHERE "+name+" IS NULL)").Scan(&valid); err != nil {
					return err
				}
				if !valid {
					quality = false
				}
			}
		}
		if len(keys) > 0 {
			filter := ""
			if step.Strategy == "scd2" {
				filter = " WHERE _is_current"
			}
			var valid bool
			if err = tx.QueryRow(ctx, "SELECT NOT EXISTS(SELECT 1 FROM "+target+filter+" GROUP BY "+strings.Join(keys, ",")+" HAVING count(*)>1)").Scan(&valid); err != nil {
				return err
			}
			if !valid {
				quality = false
			}
		}
		for _, check := range step.Checks {
			valid := true
			switch check.Kind {
			case "row_count":
				valid = state.Rows >= check.Minimum && (check.Maximum == 0 || state.Rows <= check.Maximum)
			case "not_null":
				err = tx.QueryRow(ctx, "SELECT NOT EXISTS(SELECT 1 FROM "+target+" WHERE "+pgx.Identifier{check.Column}.Sanitize()+" IS NULL)").Scan(&valid)
			case "unique":
				err = tx.QueryRow(ctx, "SELECT NOT EXISTS(SELECT 1 FROM "+target+" GROUP BY "+pgx.Identifier{check.Column}.Sanitize()+" HAVING count(*)>1)").Scan(&valid)
			default:
				return ErrInvalid
			}
			if err != nil {
				return err
			}
			if !valid {
				quality = false
			}
		}
		registry := pgx.Identifier{stage.Schema, "_pipeline_stages"}.Sanitize()
		if _, err = tx.Exec(ctx, "UPDATE "+registry+" SET table_oid=$3 WHERE operation_id=$1 AND step_id=$2", stage.Operation, stage.Step, oid); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, "GRANT SELECT ON "+target+" TO "+pgx.Identifier{r.User}.Sanitize()); err != nil {
			return err
		}
		if !quality {
			state.State = "quality_failed"
			stage.State = "quality_failed"
			state.Code = "quality_failed"
			return nil
		}
		stage.State = "checked"
		*stage = sources.SealPipelineStage(*stage)
		state.State = "checked"
		return nil
	})
	if err == nil && !quality {
		return state, ErrPipelineQuality
	}
	return state, err
}

func (s *PipelineService) executePipelineStage(ctx context.Context, inv jobs.Invocation, record PipelineRecord, c config.SourceConnection, state PipelineStageState, step PipelineStep, sql string, b readexec.Binding) error {
	e, err := inv.Current("pipeline.run", record.Definition.ID, record.Digest)
	if err != nil {
		return err
	}
	if err = access.Require(e, "engineering.pipeline.run", access.Resource{Tenant: e.Tenant(), Kind: "source", Permission: "write", ID: state.Stage.Source}, access.Resource{Tenant: e.Tenant(), Kind: "execution_context", Permission: "use", ID: state.Stage.Context}); err != nil {
		return err
	}
	if previous := state.Previous; previous != nil && step.Strategy != "replace" {
		if !previous.Valid() || previous.Tenant != e.Tenant() || previous.Pipeline != record.Definition.ID || previous.Step != step.ID || previous.Alias != c.ID || previous.Schema != state.Stage.Schema {
			return ErrOwnership
		}
		if err = access.Require(e, "engineering.pipeline.run", access.Resource{Tenant: e.Tenant(), Kind: "source", Permission: "query", ID: previous.Source}, access.Resource{Tenant: e.Tenant(), Kind: "execution_context", Permission: "use", ID: previous.Context}); err != nil {
			return err
		}
	}
	w, _, err := managedWriterConfig(s.values, s.lookup, s.repo.DatabaseName(), c)
	if err != nil {
		return err
	}
	if err = sources.RequirePipelineReadLocation(ctx, w); err != nil {
		return err
	}
	prior := state
	if state.Application != "" {
		state.PriorApplications = append(append([]string(nil), state.PriorApplications...), state.Application)
	}
	state.Input = b
	state.Dependencies = append([]string(nil), step.Inputs...)
	state.State = "dispatching"
	state.Stage.State = "dispatching"
	state.Stage.Digest = ""
	state.Fence = inv.Lease().Fence
	state.Application = "cw-pipeline-" + readexec.Hash([]string{state.Stage.Operation, state.Stage.Step, strconv.FormatInt(state.Fence, 10)})[:32]
	if _, err = s.repo.MutatePipelineStage(ctx, inv, record, state); err != nil {
		return err
	}
	fresh, compensated, err := s.preparePipelineStage(ctx, c, record, prior, b, step.Inputs)
	if err != nil {
		return err
	}
	state.Compensated = compensated
	inputs := []readexec.Relation{}
	for _, id := range step.Inputs {
		for _, relation := range b.Relations {
			if relation.ID == id {
				inputs = append(inputs, relation)
			}
		}
	}
	receipt, err := runPipelineAsset(ctx, s.values.Pipelines, step, pipelineRunnerTarget{Schema: state.Stage.Schema, Table: state.Stage.Table, Application: state.Application, Inputs: inputs}, sql, w, fresh)
	if err != nil {
		return err
	}
	state.RenderedHash = receipt.RenderedHash
	state.ValidationHash = receipt.ValidationHash
	state.LineageHash = receipt.LineageHash
	state, err = s.inspectPipelineStage(ctx, c, step, state)
	if err != nil {
		if errors.Is(err, ErrPipelineQuality) {
			if _, saveErr := s.repo.MutatePipelineStage(ctx, inv, record, state); saveErr != nil {
				return saveErr
			}
		}
		return fmt.Errorf("pipeline check: %w", err)
	}
	_, err = s.repo.MutatePipelineStage(ctx, inv, record, state)
	return err
}

// validatePipelineDestination performs no warehouse mutations. An existing
// namespace needs the actual private workspace marker; a configured prefix is
// insufficient. The execution path repeats this proof under its writer lock.
func (s *PipelineService) validatePipelineDestination(ctx context.Context, e identity.Envelope, alias string) error {
	c, err := s.connection(e, alias)
	if err != nil {
		return err
	}
	w, r, err := managedWriterConfig(s.values, s.lookup, s.repo.DatabaseName(), c)
	if err != nil {
		return err
	}
	check, stop := context.WithTimeout(ctx, time.Duration(s.values.Sources.QueryTimeout))
	defer stop()
	conn, err := pgx.ConnectConfig(check, w)
	if err != nil {
		return ErrUnavailable
	}
	defer func() {
		clean, end := context.WithTimeout(context.WithoutCancel(check), time.Second)
		defer end()
		_ = conn.Close(clean)
	}()
	tx, err := conn.BeginTx(check, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		return ErrUnavailable
	}
	defer func() {
		clean, end := context.WithTimeout(context.WithoutCancel(check), time.Second)
		defer end()
		_ = tx.Rollback(clean)
	}()
	schema, _, err := sources.ManagedLocation(c, "pipeline")
	if err != nil {
		return ErrOwnership
	}
	var oid, owner int64
	err = tx.QueryRow(check, `SELECT oid::bigint,nspowner::bigint FROM pg_catalog.pg_namespace WHERE nspname=$1`, schema).Scan(&oid, &owner)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return ErrUnavailable
	}
	var expected int64
	var unsafe bool
	if err = tx.QueryRow(check, `SELECT oid::bigint,rolsuper OR rolbypassrls OR rolcreaterole OR rolcreatedb OR current_user<>session_user FROM pg_catalog.pg_roles WHERE rolname=current_user`).Scan(&expected, &unsafe); err != nil {
		return ErrUnavailable
	}
	if unsafe || owner != expected {
		return ErrOwnership
	}
	marker := pgx.Identifier{schema, "_chartworks_workspace"}.Sanitize()
	var valid bool
	if err = tx.QueryRow(check, `SELECT c.relowner=$2::oid AND c.relkind='r' AND NOT(c.relrowsecurity OR c.relforcerowsecurity OR c.relispartition) AND NOT EXISTS(SELECT 1 FROM pg_catalog.pg_trigger WHERE tgrelid=c.oid AND NOT tgisinternal) AND NOT EXISTS(SELECT 1 FROM pg_catalog.pg_rewrite WHERE ev_class=c.oid) FROM pg_catalog.pg_class c WHERE c.oid=to_regclass($1)`, marker, expected).Scan(&valid); err != nil || !valid {
		return ErrOwnership
	}
	if _, err = tx.Exec(check, "LOCK TABLE "+marker+" IN ACCESS SHARE MODE"); err != nil {
		return ErrOwnership
	}
	var tenant, connection, writer, reader string
	var schemaOID int64
	if err = tx.QueryRow(check, "SELECT tenant_id,connection_id,schema_oid,writer_role,reader_role FROM "+marker+" WHERE singleton=true").Scan(&tenant, &connection, &schemaOID, &writer, &reader); err != nil {
		return ErrOwnership
	}
	if tenant != c.Tenant || connection != c.ID || schemaOID != oid || writer != w.User || reader != r.User {
		return ErrOwnership
	}
	return nil
}

func ownedPipelineReadTable(ctx context.Context, tx pgx.Tx, schema, table string, expected int64) (int64, error) {
	name := pgx.Identifier{schema, table}.Sanitize()
	for pass := 0; pass < 2; pass++ {
		var oid int64
		var safe bool
		if err := tx.QueryRow(ctx, `SELECT c.oid::bigint,c.relkind='r' AND c.relowner=(SELECT oid FROM pg_catalog.pg_roles WHERE rolname=current_user) AND NOT(c.relrowsecurity OR c.relforcerowsecurity OR c.relispartition) AND NOT EXISTS(SELECT 1 FROM pg_catalog.pg_trigger WHERE tgrelid=c.oid AND NOT tgisinternal) AND NOT EXISTS(SELECT 1 FROM pg_catalog.pg_rewrite WHERE ev_class=c.oid) AND NOT EXISTS(SELECT 1 FROM pg_catalog.pg_inherits WHERE inhrelid=c.oid OR inhparent=c.oid) FROM pg_catalog.pg_class c WHERE c.oid=to_regclass($1)`, name).Scan(&oid, &safe); err != nil || !safe || oid != expected {
			return 0, ErrOwnership
		}
		if pass == 0 {
			if _, err := tx.Exec(ctx, "LOCK TABLE "+name+" IN ACCESS SHARE MODE"); err != nil {
				return 0, err
			}
		}
	}
	return expected, nil
}
