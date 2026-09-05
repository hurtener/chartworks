from pathlib import Path

root = Path.cwd()
def replace(path, old, new, count=1):
    p = root/path
    s=p.read_text()
    assert s.count(old)==count, (path, old, s.count(old))
    p.write_text(s.replace(old,new))

for path in ['internal/exec/validator.go','internal/sources/sources.go','internal/vindex/vindex.go']:
    replace(path,'reflect.Ptr','reflect.Pointer')
replace('internal/sources/postgres.go','mode != "verify-full" && !(loopback && mode == "disable")','mode != "verify-full" && (!loopback || mode != "disable")')
p=root/'internal/exec/parser_test.go'
s=p.read_text()
for old,new in [
 ('([]string, error, string)', '([]string, string, error)'),
 ('return nil, err, ""', 'return nil, "", err'),
 ('return nil, err, raw', 'return nil, raw, err'),
 ('return nil, ErrUnsafe, raw', 'return nil, raw, ErrUnsafe'),
 ('return cols, err, raw', 'return cols, raw, err'),
 ('cols, err, raw := resolveFixture(sql)', 'cols, raw, err := resolveFixture(sql)'),
 ('_, err, _ := resolveFixture(sql)', '_, _, err := resolveFixture(sql)'),
]:
    assert old in s, (str(p),old)
    s=s.replace(old,new)
p.write_text(s)
replace('test/acceptance/phase08_test.go','"context"\n', '"context"\n\t"crypto/rand"\n')
replace('test/acceptance/phase08_test.go','newPassword := "ROTATED_SYNTHETIC_PASSWORD"', '''var nonce [16]byte
        if _, err := rand.Read(nonce[:]); err != nil { t.Fatal(err) }
        newPassword := fmt.Sprintf("%x", nonce[:])''')
p=root/'internal/exec/contracts.go'
s=p.read_text()
assert 'func (p *Plan) UnmarshalJSON(' not in s
s+='''
// UnmarshalJSON rejects all reconstruction from serialized data. A failed attempt
// also clears any existing plan, so its previous authority cannot survive a decode.
func (p *Plan) UnmarshalJSON([]byte) error {
    if p != nil { *p = Plan{} }
    return ErrBinding
}
'''
p.write_text(s)
replace('test/acceptance/phase08_test.go', '''if err := json.Unmarshal([]byte(`{"SQL":"DELETE FROM analytics.sales","nativeChecked":true}`), &forged); err != nil {
\t\t\tt.Fatal(err)
\t\t}''','''if err := json.Unmarshal([]byte(`{"SQL":"DELETE FROM analytics.sales","nativeChecked":true}`), &forged); !errors.Is(err, readexec.ErrBinding) {
            t.Fatal("serialized data reconstructed a plan", err)
        }
        if err := (*readexec.Plan)(nil).UnmarshalJSON([]byte(`{}`)); !errors.Is(err, readexec.ErrBinding) {
            t.Fatal("nil reconstruction accepted", err)
        }''')
replace('test/acceptance/phase08_test.go', '''if rows, err := f.s.Read(ctx, f.e, plan); err != nil || len(rows.Values) != 1 {
\t\t\tt.Fatal("valid plan failed", err)
\t\t}''','''if rows, err := f.s.Read(ctx, f.e, plan); err != nil || len(rows.Values) != 1 {
            t.Fatal("valid plan failed", err)
        }
        forged = plan
        if err := json.Unmarshal([]byte(`{}`), &forged); !errors.Is(err, readexec.ErrBinding) {
            t.Fatal("existing plan reconstruction accepted", err)
        }
        before = f.lookups.Load()
        if _, err := f.s.Read(ctx, f.e, forged); !errors.Is(err, readexec.ErrBinding) || before != f.lookups.Load() {
            t.Fatal("failed decode retained previous plan authority", err)
        }''')
