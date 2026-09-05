package acceptance

import (
 "context"
 "errors"
 "testing"

 "github.com/hurtener/chartworks/internal/store"
 "github.com/hurtener/chartworks/internal/store/postgres"
 "github.com/hurtener/chartworks/test/support"
)

func TestMissingRelationFailsReadiness(t *testing.T){
 dsn:=support.Database(t);db:=support.Open(t,dsn);raw:=support.Raw(t,dsn)
 sql(t,raw,`ALTER TABLE chartworks.operations RENAME TO missing_operations`)
 if e:=db.Check(context.Background());!errors.Is(e,store.ErrMigration){t.Fatal("history alone masked missing relation")}
 if _,e:=postgres.Open(context.Background(),dsn,postgres.Defaults());!errors.Is(e,store.ErrMigration){t.Fatal("startup accepted missing relation")}
 for _,dsn:=range []string{"","   "}{if _,e:=postgres.Open(context.Background(),dsn,postgres.Defaults());!errors.Is(e,store.ErrInvalid){t.Fatal("ambient DSN accepted")}}
}
