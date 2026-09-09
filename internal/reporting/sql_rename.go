package reporting

import (
	"context"
	"strings"
)

func quoteSQLIdentifier(s string) string {return `"`+strings.ReplaceAll(s,`"`,`""`)+`"`}

// identifierSpan consumes a single grammar-selected identifier, not a search for
// matching words in comments, string literals or other expression namespaces.
func identifierSpan(sql string,start int)(int,string,error){
	if start<0 || start>=len(sql){return 0,"",ErrInvalid}
	if sql[start]=='"'{
		var value strings.Builder
		for i:=start+1;i<len(sql);i++{
			if sql[i]!='"'{value.WriteByte(sql[i]);continue}
			if i+1<len(sql) && sql[i+1]=='"'{value.WriteByte('"');i++;continue}
			return i+1,value.String(),nil
		}
		return 0,"",ErrInvalid
	}
	i:=start
	for i<len(sql){c:=sql[i];if c>='a'&&c<='z' || c>='A'&&c<='Z' || c=='_' || i>start && (c>='0'&&c<='9' || c=='$'){i++;continue};break}
	if i==start{return 0,"",ErrInvalid}
	return i,strings.ToLower(sql[start:i]),nil
}

func columnSpan(sql string,column map[string]any,names []string)(int,int,error){
	position,ok:=astInteger(column["location"]);if !ok{return 0,0,ErrInvalid}
	lastStart,lastEnd:=position,position
	for i,wanted:=range names{
		for position<len(sql) && strings.ContainsRune(" \t\n\r",rune(sql[position])){position++}
		lastStart=position
		end,actual,err:=identifierSpan(sql,position)
		if err!=nil || actual!=wanted{return 0,0,ErrInvalid}
		lastEnd,position=end,end
		if i==len(names)-1{continue}
		for position<len(sql) && strings.ContainsRune(" \t\n\r",rune(sql[position])){position++}
		if position>=len(sql) || sql[position]!='.'{return 0,0,ErrInvalid};position++
	}
	return lastStart,lastEnd,nil
}

// renameSQL deliberately limits automatic assistance to one unambiguous base
// relation. Joins, CTEs, nested selects, stars and colliding aliases require a
// manual amendment followed by the same real validation lifecycle.
func renameSQL(ctx context.Context,sql string,renames []Rename,dependencies []Dependency)(string,error){
	if len(renames)==0 || len(renames)>256{return "",ErrInvalid}
	document,root,err:=parseAssistance(ctx,sql);if err!=nil{return "",err}
	from:=astArray(root["fromClause"])
	if len(from)!=1 || root["withClause"]!=nil || root["larg"]!=nil || root["rarg"]!=nil{return "",ErrInvalid}
	rangeVar:=astObject(astObject(from[0])["RangeVar"])
	if rangeVar==nil{return "",ErrInvalid}
	schema,table:=astString(rangeVar["schemaname"]),astString(rangeVar["relname"])
	alias:=astString(astObject(rangeVar["alias"])["aliasname"])
	if alias==""{alias=astString(astObject(astObject(rangeVar["alias"])["Alias"])["aliasname"])}
	if schema=="" || table==""{return "",ErrInvalid}
	dataset:=""
	for _,dep:=range dependencies{if dep.Schema==schema && dep.Name==table{if dataset!="" && dataset!=dep.Dataset{return "",ErrInvalid};dataset=dep.Dataset}}
	if dataset==""{return "",ErrInvalid}
	mapping:=map[string]string{}
	for _,rename:=range renames{
		if rename.Dataset!=dataset || rename.From=="" || rename.To=="" || rename.From==rename.To || !text(rename.To,128){return "",ErrInvalid}
		if old,ok:=mapping[rename.From];ok && old!=rename.To{return "",ErrInvalid};mapping[rename.From]=rename.To
	}
	selects,ranges:=0,0
	budget:=20000
	if err:=walkAST(document,0,&budget,func(m map[string]any)error{
		if m["SelectStmt"]!=nil{selects++};if m["RangeVar"]!=nil{ranges++}
		if m["SubLink"]!=nil || m["A_Star"]!=nil{return ErrInvalid};return nil
	});err!=nil || selects!=1 || ranges!=1{return "",ErrInvalid}
	qualifies:=func(names []string)bool{
		switch len(names){case 1:return true;case 2:if alias!=""{return names[0]==alias};return names[0]==table;case 3:return alias=="" && names[0]==schema && names[1]==table;default:return false}
	}
	edits:=[]sqlEdit{}
	for _,target:=range astArray(root["targetList"]){
		res:=astObject(astObject(target)["ResTarget"])
		outputName:=astString(res["name"])
		names,plainColumn:=columnNames(res["val"])
		if outputName!="" && mapping[outputName]!="" && (!plainColumn || names[len(names)-1]!=outputName){return "",ErrInvalid}
		if !plainColumn || !qualifies(names) || mapping[names[len(names)-1]]=="" || outputName!=""{continue}
		_,end,err:=columnSpan(sql,astObject(astObject(res["val"])["ColumnRef"]),names);if err!=nil{return "",err}
		name:=names[len(names)-1]
		res["name"]=name
		edits=append(edits,sqlEdit{start:end,end:end,text:" AS "+quoteSQLIdentifier(name)})
	}
	budget=20000
	err=walkAST(document,0,&budget,func(m map[string]any)error{
		column:=astObject(m["ColumnRef"]);if column==nil{return nil}
		names,ok:=columnNames(m);if !ok || !qualifies(names){return ErrInvalid}
		replacement:=mapping[names[len(names)-1]];if replacement==""{return nil}
		start,end,err:=columnSpan(sql,column,names);if err!=nil{return err}
		fields:=astArray(column["fields"])
		astObject(astObject(fields[len(fields)-1])["String"])["sval"]=replacement
		edits=append(edits,sqlEdit{start:start,end:end,text:quoteSQLIdentifier(replacement)})
		return nil
	})
	if err!=nil{return "",err}
	if len(edits)==0{return sql,nil}
	return applySQLEdits(ctx,sql,document,edits)
}
