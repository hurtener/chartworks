from pathlib import Path

def change(path,old,new):
 p=Path(path);s=p.read_text()
 if s.count(old)!=1: raise SystemExit('expected single edit: '+path)
 p.write_text(s.replace(old,new))
change('test/acceptance/phase01_test.go','t.Run("AC02", func(t *testing.T) {','t.Run("AC02", func(t *testing.T) {\n checkRealStartup(t)')
change('test/acceptance/phase02_test.go','t.Run("AC05", func(t *testing.T) {','t.Run("AC05", func(t *testing.T) {\n checkOperationExpiry(t)')
p=Path('test/acceptance/adversarial_test.go');s=p.read_text().replace('if e=store.Policy{}.Validate();','if e=(store.Policy{}).Validate();').replace(' "github.com/hurtener/chartworks/internal/config"\n','').replace('var _ config.Duration=config.Duration(time.Second)\n','');p.write_text(s)
