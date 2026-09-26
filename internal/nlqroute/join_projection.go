package nlqroute

import (
	"context"

	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/nlq"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/semantics/topics"
)

// attachJoinProjection carries the actual confirmed choices into mandatory
// rendering. It neither selects output concepts nor changes admission, rules,
// answer_context, analytical capability, or the complete validation relations.
func attachJoinProjection(ctx context.Context, input *nlq.ContextInput, admitted []admittedTopic, choices []JoinChoice) error {
	if ctx == nil || input == nil {
		return readexec.ErrBinding
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(admitted) < 2 || len(input.Relations) == 0 {
		return nil
	}
	if err := confirmJoins(admitted, choices); err != nil {
		return err
	}
	var staged []nlq.MandatoryConstraint
	for _, item := range admitted {
		if err := ctx.Err(); err != nil {
			return err
		}
		var chosen semantics.Join
		for _, choice := range choices {
			if choice.Topic == item.id {
				for _, join := range item.publication.Definition.Joins {
					if join.ID == choice.JoinID {
						chosen = join
					}
				}
			}
		}
		left, err := projectionJoinColumn(item.publication.Definition, chosen.Left)
		if err != nil {
			return err
		}
		right, err := projectionJoinColumn(item.publication.Definition, chosen.Right)
		if err != nil {
			return err
		}
		var owned []nlq.SourceRelation
		for _, r := range input.Relations {
			if r.Topic == item.id {
				owned = append(owned, r)
			}
		}
		c, err := nlq.JoinProjectionConstraint(nlq.ConfirmedJoinProjection{Version: "confirmed-joins-v1", Topic: item.id, ID: chosen.ID, Type: string(chosen.Type), Cardinality: string(chosen.Cardinality), Left: left, Right: right}, owned)
		if err != nil {
			return err
		}
		staged = append(staged, c)
	}
	// Stage atomically: incomplete or foreign endpoints never mutate the input.
	result := nlq.ConstraintState{Allowed: true}
	if input.Constraints != nil {
		result = *input.Constraints
		result.Required = append([]nlq.MandatoryConstraint(nil), result.Required...)
		result.Excluded = append([]nlq.MandatoryConstraint(nil), result.Excluded...)
	}
	if len(result.Required)+len(result.Excluded)+len(staged) > nlq.MaxConstraints {
		return nlq.ErrInsufficient
	}
	result.Required = append(result.Required, staged...)
	input.Constraints = &result
	return nil
}

func projectionJoinColumn(def topics.Definition, ref semantics.Reference) (nlq.JoinProjectionColumn, error) {
	if !ref.Valid() || ref.Kind != semantics.KindColumn {
		return nlq.JoinProjectionColumn{}, readexec.ErrBinding
	}
	var out nlq.JoinProjectionColumn
	found := 0
	for _, dataset := range def.Datasets {
		if dataset.ID == ref.Dataset {
			for _, col := range dataset.Columns {
				if col.ID == ref.ID {
					out = nlq.JoinProjectionColumn{Dataset: dataset.ID, ID: col.ID, Name: col.SourceName}
					found++
				}
			}
		}
	}
	if found != 1 {
		return nlq.JoinProjectionColumn{}, readexec.ErrBinding
	}
	return out, nil
}
