package sources

import (
	"net"
	"net/url"

	"github.com/hurtener/chartworks/internal/store"
	"github.com/jackc/pgx/v5"
)

// ParseApprovedDSN validates operator-resolved PostgreSQL credentials for both
// separate read and managed-write adapters. It grants no identity authority.
// Never log the returned config or caller's credential string.
func ParseApprovedDSN(dsn string)(*pgx.ConnConfig,string,error){
	if len(dsn)==0||len(dsn)>16384{return nil,"",store.ErrInvalid}
	u,err:=url.Parse(dsn)
	if err!=nil||(u.Scheme!="postgres"&&u.Scheme!="postgresql")||u.User==nil||u.User.Username()==""||u.Hostname()==""||u.Path==""||u.Path=="/"||u.Fragment!=""{return nil,"",store.ErrInvalid}
	password,has:=u.User.Password();if !has||password==""||len(password)>8192{return nil,"",store.ErrInvalid}
	for key,values:=range u.Query(){
		if len(values)!=1{return nil,"",store.ErrInvalid}
		switch key{case "sslmode","sslrootcert","sslcert","sslkey":default:return nil,"",store.ErrInvalid}
	}
	mode:=u.Query().Get("sslmode");ip:=net.ParseIP(u.Hostname());loopback:=u.Hostname()=="localhost"||ip!=nil&&ip.IsLoopback()
	if mode!="verify-full"&&(!loopback||mode!="disable"){return nil,"",store.ErrInvalid}
	cfg,err:=pgx.ParseConfig(dsn);if err!=nil{return nil,"",store.ErrInvalid}
	return cfg,u.Host+u.EscapedPath(),nil
}
