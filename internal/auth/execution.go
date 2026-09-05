package auth

import (
 "context"
 "encoding/base64"
 "strings"

 "github.com/hurtener/chartworks/internal/identity"
)

// VerifyExecution consumes Pengui execution-authority/v1. It uses the same issuer/key cache,
// a dedicated audience, zero expiry leeway and a maximum sixty-second lifetime.
// A JWT for another job, binding, manifest or ordinary surface cannot authorize this attempt.
func(v *Verifier)VerifyExecution(ctx context.Context,token,binding,job,digest string,revision int64)(identity.Envelope,error){
 e,err:=v.Verify(ctx,token,Execution);if err!=nil{return identity.Envelope{},ErrToken}
 parts:=strings.Split(token,".");body,err:=base64.RawURLEncoding.Strict().DecodeString(parts[1]);if err!=nil{return identity.Envelope{},ErrToken}
 claims,err:=Object(body,v.cfg.MaxClaimBytes);if err!=nil{return identity.Envelope{},ErrToken}
 version,vok:=number(claims,"execution_version");id,iok:=stringValue(claims,"execution_binding");manifest,mok:=stringValue(claims,"execution_manifest");rev,rok:=number(claims,"execution_binding_revision")
 if !vok||version!=1||!iok||id!=binding||!mok||manifest!=digest||!rok||rev!=revision||e.Session()!=job||e.User()!="svc:chartworks:"+binding{return identity.Envelope{},ErrToken}
 return e,nil
}
