package main

import (
 "os"
 "testing"
)

func TestRunVersion(t *testing.T){old:=os.Args;defer func(){os.Args=old}();os.Args=[]string{"chartworks","version"};if run()!=0{t.Fatal("version failed")}}
