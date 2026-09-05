package exec

import (
	"fmt"
	"strings"
)

// The parser is PostgreSQL's grammar compiled to WASM. This resolver is deliberately
// positive: every executable expression, dependency and column must be understood.
// Unknown nodes/fields do not inherit trust from a SELECT ancestor or EXPLAIN success.

type sqlResolver struct {
	binding Binding
	dependencies map[string]bool
	parameters map[int]bool
	parameterCount int
}
type sqlScope struct {
	sources map[string][]string
	ctes map[string][]string
	outputs []string
	windows map[string]bool
}
func object(v any) map[string]any { m,_:=v.(map[string]any); return m }
func array(v any) []any { a,_:=v.([]any); return a }
func text(v any) string { s,_:=v.(string); return s }
func truth(v any) bool { b,_:=v.(bool); return b }
func fieldObject(v any,kind string) map[string]any {
	m:=object(v)
	if wrapped,ok:=m[kind]; ok && len(m)==1 { return object(wrapped) }
	return m
}
func only(m map[string]any,allowed ...string) bool {
	if m==nil { return false }
	for key:=range m {
		if key=="location" { continue }
		found:=false; for _,a:=range allowed { if key==a { found=true; break } }
		if !found { return false }
	}
	return true
}
func named(v any) (string,bool) {
	m:=object(v)
	if len(m)!=1 { return "",false }
	s,ok:=m["String"]
	if !ok { return "",false }
	value:=object(s)
	if !only(value,"sval") { return "",false }
	n:=text(value["sval"])
	return n,n!=""
}
func names(v any) ([]string,bool) {
	a:=array(v); out:=make([]string,0,len(a))
	for _,item:=range a { n,ok:=named(item); if !ok { return nil,false }; out=append(out,n) }
	return out,true
}
func copyCTEs(in map[string][]string) map[string][]string {
	out:=make(map[string][]string,len(in))
	for name,cols:=range in { out[name]=append([]string(nil),cols...) }
	return out
}

func (r *sqlResolver) selectStatement(v any,inherited map[string][]string) ([]string,error) {
	root:=object(v)
	if len(root)!=1 || root["SelectStmt"]==nil { return nil,ErrUnsafe }
	m:=object(root["SelectStmt"])
	if !only(m,"distinctClause","targetList","fromClause","whereClause","groupClause","groupDistinct","havingClause","windowClause","valuesLists","sortClause","limitOffset","limitCount","limitOption","op","all","larg","rarg","withClause") { return nil,ErrUnsafe }
	scope:=&sqlScope{sources:map[string][]string{},ctes:copyCTEs(inherited),windows:map[string]bool{}}
	if with:=m["withClause"]; with!=nil {
		w:=fieldObject(with,"WithClause")
		if !only(w,"ctes","recursive") || truth(w["recursive"]) { return nil,ErrUnsupported }
		local:=map[string]bool{}
		for _,item:=range array(w["ctes"]) {
			cte:=fieldObject(item,"CommonTableExpr")
			if !only(cte,"ctename","aliascolnames","ctematerialized","ctequery") { return nil,ErrUnsafe }
			name:=text(cte["ctename"])
			if !SQLIdentifier(name) || local[name] { return nil,ErrUnsafe }
			cols,err:=r.selectStatement(cte["ctequery"],scope.ctes); if err!=nil { return nil,err }
			aliases,ok:=names(cte["aliascolnames"]); if !ok || len(aliases)>len(cols) { return nil,ErrUnsafe }
			for i,alias:=range aliases { if !SQLIdentifier(alias) { return nil,ErrUnsupported }; cols[i]=alias }
			scope.ctes[name]=cols; local[name]=true
		}
	}
	op:=text(m["op"])
	if op!="" && op!="SETOP_NONE" {
		if op!="SETOP_UNION" && op!="SETOP_INTERSECT" && op!="SETOP_EXCEPT" { return nil,ErrUnsupported }
		if len(array(m["targetList"]))!=0 || len(array(m["fromClause"]))!=0 || m["whereClause"]!=nil || m["havingClause"]!=nil || len(array(m["groupClause"]))!=0 || len(array(m["valuesLists"]))!=0 { return nil,ErrUnsafe }
		left,err:=r.selectStatement(map[string]any{"SelectStmt":m["larg"]},scope.ctes); if err!=nil { return nil,err }
		right,err:=r.selectStatement(map[string]any{"SelectStmt":m["rarg"]},scope.ctes); if err!=nil { return nil,err }
		if len(left)!=len(right) { return nil,ErrUnsafe }
		scope.outputs=left
		if err=r.trailing(m,scope); err!=nil { return nil,err }
		return left,nil
	}
	if m["larg"]!=nil || m["rarg"]!=nil { return nil,ErrUnsafe }
	for _,item:=range array(m["fromClause"]) { if err:=r.from(item,scope); err!=nil { return nil,err } }
	for _,item:=range array(m["windowClause"]) {
		window:=fieldObject(item,"WindowDef"); name:=text(window["name"])
		if !SQLIdentifier(name) || scope.windows[name] { return nil,ErrUnsafe }; scope.windows[name]=true
	}
	if values:=array(m["valuesLists"]); len(values)>0 {
		if len(scope.sources)>0 || len(array(m["targetList"]))>0 { return nil,ErrUnsafe }
		width:=0
		for _,row:=range values {
			items:=array(fieldObject(row,"List")["items"])
			if len(items)==0 { return nil,ErrUnsafe }
			if width!=0 && width!=len(items) { return nil,ErrUnsafe }; width=len(items)
			for _,item:=range items { if err:=r.expr(item,scope,false); err!=nil { return nil,err } }
		}
		for i:=0;i<width;i++ { scope.outputs=append(scope.outputs,fmt.Sprintf("column%d",i+1)) }
	} else {
		targets:=array(m["targetList"])
		if len(targets)<1 || len(targets)>256 { return nil,ErrLimit }
		for _,item:=range targets {
			target:=fieldObject(item,"ResTarget")
			if !only(target,"name","val") { return nil,ErrUnsafe }
			if err:=r.expr(target["val"],scope,false); err!=nil { return nil,err }
			name:=text(target["name"])
			if name!="" && !SQLIdentifier(name) { return nil,ErrUnsupported }
			if name=="" { name=resultName(target["val"]) }
			scope.outputs=append(scope.outputs,name)
		}
	}
	for _,field:=range []string{"whereClause","havingClause"} { if err:=r.optional(m[field],scope,false); err!=nil { return nil,err } }
	for _,field:=range []string{"groupClause","distinctClause"} {
		for _,item:=range array(m[field]) {
			if item==nil && field=="distinctClause" { continue }
			if err:=r.expr(item,scope,true); err!=nil { return nil,err }
		}
	}
	for _,item:=range array(m["windowClause"]) { if err:=r.window(fieldObject(item,"WindowDef"),scope); err!=nil { return nil,err } }
	if err:=r.trailing(m,scope); err!=nil { return nil,err }
	return append([]string(nil),scope.outputs...),nil
}
func resultName(v any) string {
	m:=object(v)
	if col:=object(m["ColumnRef"]); col!=nil { parts,ok:=names(col["fields"]); if ok && len(parts)>0 { return parts[len(parts)-1] } }
	if f:=object(m["FuncCall"]); f!=nil { parts,ok:=names(f["funcname"]); if ok && len(parts)>0 { return parts[len(parts)-1] } }
	return "?column?"
}
func (r *sqlResolver) trailing(m map[string]any,s *sqlScope) error {
	for _,item:=range array(m["sortClause"]) { if err:=r.expr(item,s,true); err!=nil { return err } }
	for _,field:=range []string{"limitCount","limitOffset"} { if err:=r.optional(m[field],s,false); err!=nil { return err } }
	option:=text(m["limitOption"])
	if option!="" && option!="LIMIT_OPTION_DEFAULT" && option!="LIMIT_OPTION_COUNT" && option!="LIMIT_OPTION_WITH_TIES" { return ErrUnsupported }
	return nil
}
func addSource(s *sqlScope,name string,columns []string,alias any) error {
	if alias!=nil {
		a:=fieldObject(alias,"Alias")
		if !only(a,"aliasname","colnames") { return ErrUnsafe }
		name=text(a["aliasname"])
		n,ok:=names(a["colnames"]); if !ok || len(n)>len(columns) { return ErrUnsafe }
		columns=append([]string(nil),columns...)
		for i,col:=range n { if !SQLIdentifier(col) { return ErrUnsupported }; columns[i]=col }
	}
	if !SQLIdentifier(name) || s.sources[name]!=nil { return ErrUnsafe }
	s.sources[name]=append([]string(nil),columns...)
	return nil
}
func (r *sqlResolver) from(v any,s *sqlScope) error {
	root:=object(v)
	if len(root)!=1 { return ErrUnsafe }
	if value,ok:=root["RangeVar"]; ok {
		m:=object(value)
		if !only(m,"catalogname","schemaname","relname","inh","relpersistence","alias") || text(m["catalogname"])!="" { return ErrUnsafe }
		schema,name:=text(m["schemaname"]),text(m["relname"])
		if schema=="" {
			cols,ok:=s.ctes[name]; if !ok { return ErrUnsafe }
			return addSource(s,name,cols,m["alias"])
		}
		for _,relation:=range r.binding.Relations {
			if relation.Schema!=schema || relation.Name!=name { continue }
			columns:=[]string{}
			for _,column:=range relation.Columns { if column.Safe { columns=append(columns,column.Name) } }
			if len(columns)==0 { return ErrUnsupported }
			r.dependencies[relation.ID]=true
			return addSource(s,name,columns,m["alias"])
		}
		return ErrUnsafe
	}
	if value,ok:=root["RangeSubselect"]; ok {
		m:=object(value)
		if !only(m,"lateral","subquery","alias") || truth(m["lateral"]) || m["alias"]==nil { return ErrUnsupported }
		columns,err:=r.selectStatement(m["subquery"],s.ctes); if err!=nil { return err }
		return addSource(s,"",columns,m["alias"])
	}
	if value,ok:=root["JoinExpr"]; ok {
		m:=object(value)
		if !only(m,"jointype","isNatural","larg","rarg","quals","rtindex") || truth(m["isNatural"]) { return ErrUnsupported }
		switch text(m["jointype"]) { case "JOIN_INNER","JOIN_LEFT","JOIN_RIGHT","JOIN_FULL": default: return ErrUnsupported }
		if err:=r.from(m["larg"],s); err!=nil { return err }
		if err:=r.from(m["rarg"],s); err!=nil { return err }
		return r.optional(m["quals"],s,false)
	}
	return ErrUnsafe
}
func columnVisible(s *sqlScope,parts []string,outputs bool) bool {
	count:=0
	if len(parts)==1 {
		for _,cols:=range s.sources { for _,c:=range cols { if c==parts[0] { count++ } } }
		if count==0 && outputs { for _,c:=range s.outputs { if c==parts[0] { count++ } } }
	} else if len(parts)==2 {
		for _,c:=range s.sources[parts[0]] { if c==parts[1] { count++ } }
	}
	return count==1
}
func safeFunction(parts []string) bool {
	if len(parts)==2 && parts[0]=="pg_catalog" { parts=parts[1:] }
	if len(parts)!=1 { return false }
	switch parts[0] {
	case "count","sum","avg","min","max","abs","round","ceil","ceiling","floor","lower","upper","length","char_length","octet_length","trim","btrim","ltrim","rtrim","substring","substr","replace","date_trunc","date_part","extract","row_number","rank","dense_rank","lag","lead","first_value","last_value","nth_value","ntile","percent_rank","cume_dist": return true
	}
	return false
}
func safeType(parts []string) bool {
	if len(parts)==2 && parts[0]=="pg_catalog" { parts=parts[1:] }
	if len(parts)!=1 { return false }
	switch parts[0] { case "int2","int4","int8","numeric","float4","float8","bool","text","varchar","bpchar","date","timestamp","timestamptz","time","timetz","interval","uuid","json","jsonb","bytea","money": return true }; return false
}
func safeOperator(v any) bool {
	parts,ok:=names(v)
	if !ok || len(parts)!=1 { return false }
	switch parts[0] { case "+","-","*","/","%","=","<>","!=","<",">","<=",">=","||","~~","!~~","~~*","!~~*": return true }; return false
}
func (r *sqlResolver) optional(v any,s *sqlScope,outputs bool) error { if v==nil { return nil }; return r.expr(v,s,outputs) }
func (r *sqlResolver) expressions(v any,s *sqlScope,outputs bool) error {
	for _,item:=range array(v) { if err:=r.expr(item,s,outputs); err!=nil { return err } }; return nil
}
func (r *sqlResolver) window(m map[string]any,s *sqlScope) error {
	if !only(m,"name","refname","partitionClause","orderClause","frameOptions","startOffset","endOffset") { return ErrUnsafe }
	if name:=text(m["refname"]); name!="" && !s.windows[name] { return ErrUnsafe }
	for _,key:=range []string{"partitionClause","orderClause"} { if err:=r.expressions(m[key],s,false); err!=nil { return err } }
	for _,key:=range []string{"startOffset","endOffset"} { if err:=r.optional(m[key],s,false); err!=nil { return err } }
	return nil
}
func (r *sqlResolver) expr(v any,s *sqlScope,outputs bool) error {
	root:=object(v)
	if len(root)!=1 { return ErrUnsafe }
	for kind,value:=range root {
		m:=object(value)
		switch kind {
		case "ColumnRef":
			if !only(m,"fields") { return ErrUnsafe }
			parts,ok:=names(m["fields"])
			if !ok || !columnVisible(s,parts,outputs) { return ErrUnsafe }
		case "ParamRef":
			if !only(m,"number") { return ErrUnsafe }
			n,ok:=m["number"].(float64)
			if !ok || n<1 || n>float64(r.parameterCount) || n!=float64(int(n)) { return ErrBinding }
			r.parameters[int(n)]=true
		case "A_Const":
			if !only(m,"ival","fval","sval","boolval","bsval","isnull") { return ErrUnsafe }
		case "A_Expr":
			if !only(m,"kind","name","lexpr","rexpr") || !safeOperator(m["name"]) { return ErrUnsafe }
			switch text(m["kind"]) {
			case "AEXPR_OP","AEXPR_OP_ANY","AEXPR_OP_ALL","AEXPR_DISTINCT","AEXPR_NOT_DISTINCT","AEXPR_IN","AEXPR_LIKE","AEXPR_ILIKE","AEXPR_BETWEEN","AEXPR_NOT_BETWEEN","AEXPR_BETWEEN_SYM","AEXPR_NOT_BETWEEN_SYM":
			default: return ErrUnsafe
			}
			if err:=r.optional(m["lexpr"],s,outputs); err!=nil { return err }
			if err:=r.optional(m["rexpr"],s,outputs); err!=nil { return err }
		case "BoolExpr":
			if !only(m,"boolop","args") { return ErrUnsafe }
			switch text(m["boolop"]) { case "AND_EXPR","OR_EXPR","NOT_EXPR": default: return ErrUnsafe }
			if err:=r.expressions(m["args"],s,outputs); err!=nil { return err }
		case "NullTest","BooleanTest":
			if !only(m,"arg","nulltesttype","argisrow","booltesttype") { return ErrUnsafe }
			if err:=r.expr(m["arg"],s,outputs); err!=nil { return err }
		case "FuncCall":
			if !only(m,"funcname","args","agg_order","agg_filter","over","agg_star","agg_distinct","agg_within_group","func_variadic","funcformat") || truth(m["func_variadic"]) { return ErrUnsafe }
			parts,ok:=names(m["funcname"]); if !ok || !safeFunction(parts) { return ErrUnsafe }
			if truth(m["agg_star"]) && (parts[len(parts)-1]!="count" || len(array(m["args"]))!=0) { return ErrUnsafe }
			for _,key:=range []string{"args","agg_order"} { if err:=r.expressions(m[key],s,outputs); err!=nil { return err } }
			if err:=r.optional(m["agg_filter"],s,outputs); err!=nil { return err }
			if m["over"]!=nil { if err:=r.window(fieldObject(m["over"],"WindowDef"),s); err!=nil { return err } }
		case "CoalesceExpr","MinMaxExpr":
			if !only(m,"args","op","coalescetype","coalescecollid","minmaxtype","minmaxcollid","inputcollid") { return ErrUnsafe }
			if err:=r.expressions(m["args"],s,outputs); err!=nil { return err }
		case "CaseExpr":
			if !only(m,"arg","args","defresult","casetype","casecollid") { return ErrUnsafe }
			if err:=r.optional(m["arg"],s,outputs); err!=nil { return err }
			if err:=r.expressions(m["args"],s,outputs); err!=nil { return err }
			if err:=r.optional(m["defresult"],s,outputs); err!=nil { return err }
		case "CaseWhen":
			if !only(m,"expr","result") { return ErrUnsafe }
			if err:=r.expr(m["expr"],s,outputs); err!=nil { return err }
			if err:=r.expr(m["result"],s,outputs); err!=nil { return err }
		case "TypeCast":
			if !only(m,"arg","typeName") { return ErrUnsafe }
			typeName:=fieldObject(m["typeName"],"TypeName")
			if !only(typeName,"names","typemod","typmods","setof","pct_type") || truth(typeName["setof"]) || truth(typeName["pct_type"]) { return ErrUnsafe }
			parts,ok:=names(typeName["names"]); if !ok || !safeType(parts) { return ErrUnsafe }
			if err:=r.expr(m["arg"],s,outputs); err!=nil { return err }
			if err:=r.expressions(typeName["typmods"],s,false); err!=nil { return err }
		case "SortBy":
			if !only(m,"node","sortby_dir","sortby_nulls","useOp") || len(array(m["useOp"]))!=0 || text(m["sortby_dir"])=="SORTBY_USING" { return ErrUnsafe }
			if err:=r.expr(m["node"],s,outputs); err!=nil { return err }
		case "List":
			if !only(m,"items") { return ErrUnsafe }
			if err:=r.expressions(m["items"],s,outputs); err!=nil { return err }
		case "A_ArrayExpr","RowExpr":
			if !only(m,"elements","args","row_format") { return ErrUnsafe }
			if err:=r.expressions(m["elements"],s,outputs); err!=nil { return err }
			if err:=r.expressions(m["args"],s,outputs); err!=nil { return err }
		case "SubLink":
			if !only(m,"subLinkType","subLinkId","testexpr","operName","subselect") { return ErrUnsafe }
			switch text(m["subLinkType"]) { case "EXISTS_SUBLINK","EXPR_SUBLINK","ANY_SUBLINK","ALL_SUBLINK": default: return ErrUnsafe }
			if len(array(m["operName"]))>0 && !safeOperator(m["operName"]) { return ErrUnsafe }
			if err:=r.optional(m["testexpr"],s,outputs); err!=nil { return err }
			if _,err:=r.selectStatement(m["subselect"],s.ctes); err!=nil { return err }
		case "SQLValueFunction":
			if !only(m,"op","typmod") { return ErrUnsafe }
			switch text(m["op"]) { case "SVFOP_CURRENT_DATE","SVFOP_CURRENT_TIME","SVFOP_CURRENT_TIME_N","SVFOP_CURRENT_TIMESTAMP","SVFOP_CURRENT_TIMESTAMP_N","SVFOP_LOCALTIME","SVFOP_LOCALTIME_N","SVFOP_LOCALTIMESTAMP","SVFOP_LOCALTIMESTAMP_N": default: return ErrUnsafe }
		case "GroupingSet":
			if !only(m,"kind","content") { return ErrUnsafe }
			if err:=r.expressions(m["content"],s,outputs); err!=nil { return err }
		default:
			return ErrUnsafe
		}
	}
	return nil
}

// ParserIdentity is a stable evidence label, not a promise that every PostgreSQL
// grammar production is executable under the initial positive safety contract.
const ParserIdentity = "libpg_query-17/pg_query_go-v6.2.2/wasm-b511bb3bfd6e"

// QualifiedName is used only for metadata comparisons, never as executable SQL.
func QualifiedName(r Relation) string { return strings.Join([]string{r.Schema,r.Name},".") }
