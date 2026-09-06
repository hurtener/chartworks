package engineering

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io/fs"
	"math/big"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/config"
	"github.com/parquet-go/parquet-go"
	pqsnap "github.com/parquet-go/parquet-go/compress/snappy"
	pqgzip "github.com/parquet-go/parquet-go/compress/gzip"
	pqzstd "github.com/parquet-go/parquet-go/compress/zstd"
	"github.com/parquet-go/parquet-go/compress"
)

func parseSpec(format string, raw []byte, columns ...UploadColumn) UploadSpec {
	s:=UploadSpec{ID:"sample",Name:"Synthetic sample",Connection:"workspace",Format:format,Columns:columns,Bytes:int64(len(raw)),SHA256:contentHash(raw)}
	if format=="xlsx" { s.Sheet="Data" }; return s
}
func parsed(t *testing.T, raw []byte, spec UploadSpec) ([]Cell,ParseReceipt) {
	t.Helper(); out:=[]Cell{}
	r,err:=Parse(context.Background(),raw,spec,config.DefaultUploads(),func(row []Cell)error{out=append(out,row...);return nil})
	if err!=nil { t.Fatal("parse",err) }; return out,r
}
func TestCSVDeclaredTypes(t *testing.T) {
	raw:=[]byte("Identificador,Importe,Activo,Nota,Fecha,Instante,Binario,Documento,Flotante\n9007199254740993,9007199254740993.125,true,,2024-02-29,2026-01-01T03:00:00+03:00,cafe,\"{\"\"n\"\":9007199254740993}\",1.25\n2,-0.01,false,\\N,2024-01-01,2024-01-01T00:00:00Z,,,0\n")
	columns:=[]UploadColumn{{"identificador","integer",false},{"importe","decimal",false},{"activo","boolean",false},{"nota","text",true},{"fecha","date",false},{"instante","timestamp",false},{"binario","binary",true},{"documento","json",true},{"flotante","number",false}}
	// Empty JSON is not NULL. Use the explicit CSV NULL marker instead.
	raw=bytes.ReplaceAll(raw,[]byte("00Z,,,0"),[]byte("00Z,,\\N,0"))
	values,r:=parsed(t,raw,parseSpec("csv",raw,columns...))
	if r.Rows!=2 || r.Cells!=18 || values[0].Text!="9007199254740993" || values[1].Text!="9007199254740993.125" || values[3].Null || values[3].Text!="" || !values[12].Null || values[5].Text!="2026-01-01T00:00:00Z" || values[7].Text!="{\"n\":9007199254740993}" { t.Fatal("exact types or NULL distinction lost",r,values) }
	for input,want:=range map[string]string{"  Año de Venta ":"ano_de_venta","\ufeffID":"id","123":"c_123","Nombre-Apellido":"nombre_apellido"} {
		got,e:=NormalizeHeader(input); if e!=nil || got!=want { t.Fatal("header normalization",input,got,e) }
	}
}
func xlsxFixture(t *testing.T, edit func(map[string]string), mode fs.FileMode) []byte {
	t.Helper()
	parts:=map[string]string{
		"[Content_Types].xml":`<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/></Types>`,
		"_rels/.rels":`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="book" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/></Relationships>`,
		"xl/workbook.xml":`<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets><sheet name="Data" sheetId="1" r:id="first"/><sheet name="Other" sheetId="2" r:id="second"/></sheets></workbook>`,
		"xl/_rels/workbook.xml.rels":`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="first" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/><Relationship Id="second" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet2.xml"/></Relationships>`,
		"xl/sharedStrings.xml":`<sst xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><si><t>id</t></si><si><r><t>no</t></r><r><t>te</t></r></si></sst>`,
		"xl/worksheets/sheet1.xml":`<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData><row r="1"><c r="A1" t="s"><v>0</v></c><c r="B1" t="s"><v>1</v></c></row><row r="2"><c r="A2"><v>9007199254740993</v></c><c r="B2" t="inlineStr"><is><t></t></is></c></row><row r="3"><c r="A3"><v>2</v></c></row></sheetData></worksheet>`,
		"xl/worksheets/sheet2.xml":`<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData><row r="1"><c r="A1" t="inlineStr"><is><t>id</t></is></c><c r="B1" t="inlineStr"><is><t>note</t></is></c></row><row r="2"><c r="A2"><v>7</v></c><c r="B2" t="inlineStr"><is><t>second</t></is></c></row></sheetData></worksheet>`,
	}
	if edit!=nil { edit(parts) }
	var b bytes.Buffer; z:=zip.NewWriter(&b)
	for name,body:=range parts { h:=&zip.FileHeader{Name:name,Method:zip.Deflate}; h.SetMode(0600); if name=="special.xml" { h.SetMode(mode) }; w,e:=z.CreateHeader(h); if e!=nil { t.Fatal(e) }; if _,e=w.Write([]byte(body));e!=nil { t.Fatal(e) } }
	if e:=z.Close();e!=nil { t.Fatal(e) };return b.Bytes()
}
func TestXLSXSelectedSheetsAndNulls(t *testing.T) {
	raw:=xlsxFixture(t,nil,0); spec:=parseSpec("xlsx",raw,UploadColumn{"id","integer",false},UploadColumn{"note","text",true})
	cells,r:=parsed(t,raw,spec)
	if r.Rows!=2 || cells[0].Text!="9007199254740993" || cells[1].Null || !cells[3].Null { t.Fatal("XLSX types/nulls",r,cells) }
	spec.Sheet="Other"; cells,r=parsed(t,raw,spec)
	if r.Rows!=1 || cells[0].Text!="7" || cells[1].Text!="second" { t.Fatal("selected wrong sheet") }
	for _,c:=range []struct{value string; epoch,stamp bool; want string}{{"1",false,false,"1900-01-01"},{"61",false,false,"1900-03-01"},{"0",true,false,"1904-01-01"},{"1.5",true,true,"1904-01-02T12:00:00Z"}} {
		got,e:=excelTemporal(c.value,c.epoch,c.stamp);if e!=nil||got!=c.want{t.Fatal("serial date",got,e)}
	}
	for _,s:=range []string{"60","NaN","-1","1.25","9999999999"} { if _,e:=excelTemporal(s,false,false);e==nil{t.Fatal("unsafe serial accepted")} }
}

type parquetFixtureRow struct {
	ID int64 `parquet:"id"`
	Name *string `parquet:"name,optional"`
	Active bool `parquet:"active"`
	Ratio float64 `parquet:"ratio"`
}
func parquetFixture(t *testing.T,codec compress.Codec) []byte {
	t.Helper(); var b bytes.Buffer; w:=parquet.NewGenericWriter[parquetFixtureRow](&b,parquet.Compression(codec)); name:="ordinary"
	if _,e:=w.Write([]parquetFixtureRow{{9007199254740993,&name,true,1.25},{-2,nil,false,0}});e!=nil{t.Fatal(e)}
	if e:=w.Close();e!=nil{t.Fatal(e)};return b.Bytes()
}
func TestParquetFlatExactTypesAndCompression(t *testing.T) {
	for name,codec:=range map[string]compress.Codec{"snappy":&pqsnap.Codec{},"gzip":&pqgzip.Codec{},"zstd":&pqzstd.Codec{}} {
		t.Run(name,func(t *testing.T){ raw:=parquetFixture(t,codec); spec:=parseSpec("parquet",raw,UploadColumn{"id","integer",false},UploadColumn{"name","text",true},UploadColumn{"active","boolean",false},UploadColumn{"ratio","number",false})
			cells:=[]Cell{}; r,e:=Parse(context.Background(),raw,spec,config.DefaultUploads(),func(row []Cell)error{cells=append(cells,row...);return nil})
			if e!=nil {
				size:=int(binary.LittleEndian.Uint32(raw[len(raw)-8:len(raw)-4])); meta,_,_:=compact(raw[len(raw)-8-size:len(raw)-8])
				for _,g:=range meta.fields[4].list { t.Log("row group",g.number(2),g.number(3));for _,c:=range g.fields[1].list{ m:=c.fields[3];at:=int(m.number(9));if m.has(11)&&m.number(11)<int64(at){at=int(m.number(11))};h,_,_:=compact(raw[at:]);t.Log("column",m.number(1),m.number(4),m.number(5),m.number(6),m.number(7),"page",h) } }
				t.Fatal("qualified Parquet rejected",e)
			}
			if r.Rows!=2 || cells[0].Text!="9007199254740993" || cells[2].Text!="true" || cells[3].Text!="1.25" || !cells[5].Null {t.Fatal("Parquet type transport",r,cells)}
		})
	}
	if scaledInteger(big.NewInt(-123),4)!="-0.0123" || scaledInteger(big.NewInt(0),2)!="0.00" {t.Fatal("decimal scale")}
}
func TestUploadParserAdversarial(t *testing.T) {
	one:=[]UploadColumn{{"id","integer",false}}
	for _,raw:=range [][]byte{[]byte("id\n9223372036854775808\n"),[]byte("id\nNaN\n"),[]byte("id\n\\N\n"),[]byte("id,id\n1,2\n"),[]byte("id\n\"bad\n"),{0xff,0},[]byte("../id\n1\n")} {
		s:=parseSpec("csv",raw,one...);if _,e:=Parse(context.Background(),raw,s,config.DefaultUploads(),func([]Cell)error{return nil});e==nil{t.Fatal("malformed CSV accepted")}
	}
	for _,value:=range []string{"=1+1","+cmd","-cmd","@SUM(1)","\t＝1"} {
		if _,e:=normalizeCell(UploadColumn{Type:"text"},Cell{Text:value});e==nil{t.Fatal("executable text accepted")}
	}
	for _,edit:=range []func(map[string]string){
		func(m map[string]string){m["../escape.xml"]="<a/>"},
		func(m map[string]string){m["xl/vbaProject.bin"]="payload"},
		func(m map[string]string){m["special.xml"]="<a/>"},
		func(m map[string]string){m["xl/worksheets/sheet2.xml"]=strings.Replace(m["xl/worksheets/sheet2.xml"],"<v>7</v>","<f>1+1</f><v>2</v>",1)},
		func(m map[string]string){m["xl/_rels/workbook.xml.rels"]=strings.Replace(m["xl/_rels/workbook.xml.rels"],"Target=","TargetMode=\"External\" Target=",1)},
		func(m map[string]string){m["xl/worksheets/sheet1.xml"]=strings.Replace(m["xl/worksheets/sheet1.xml"],"A2","XFD9999999",1)},
		func(m map[string]string){m["xl/sharedStrings.xml"]="<!DOCTYPE a [<!ENTITY x SYSTEM 'file:///x'>]><a/>"},
		func(m map[string]string){m["xl/worksheets/sheet1.xml"]=strings.Replace(m["xl/worksheets/sheet1.xml"],"<v>0</v>","<v>999999999</v>",1)},
	} { raw:=xlsxFixture(t,edit,fs.ModeSymlink|0600);s:=parseSpec("xlsx",raw,UploadColumn{"id","integer",false},UploadColumn{"note","text",true});if _,e:=Parse(context.Background(),raw,s,config.DefaultUploads(),func([]Cell)error{return nil});e==nil{t.Fatal("unsafe archive accepted")} }
	raw:=[]byte("id\n1\n2\n"); s:=parseSpec("csv",raw,one...);l:=config.DefaultUploads(); l.MaxRows=1
	if _,e:=Parse(context.Background(),raw,s,l,func([]Cell)error{return nil});!errors.Is(e,ErrLimit){t.Fatal("row bound",e)}
	s.SHA256=strings.Repeat("0",64);if _,e:=Parse(context.Background(),raw,s,l,func([]Cell)error{return nil});!errors.Is(e,ErrChecksum){t.Fatal("checksum",e)}
	s=parseSpec("csv",raw,one...);ctx,cancel:=context.WithCancel(context.Background());cancel()
	if _,e:=Parse(ctx,raw,s,l,func([]Cell)error{return nil});!errors.Is(e,context.Canceled){t.Fatal("cancellation",e)}
	if _,e:=Parse(context.Background(),raw,s,config.DefaultUploads(),func([]Cell)error{return ErrUnavailable});!errors.Is(e,ErrUnavailable){t.Fatal("consumer failure lost",e)}
	if _,e:=Parse(nil,raw,s,l,func([]Cell)error{return nil});!errors.Is(e,ErrInvalid){t.Fatal("nil context",e)}
}
func TestParquetAllocationGuards(t *testing.T) {
	for _,raw:=range [][]byte{{0x18,0xff,0xff,0xff,0xff,0x7f},bytes.Repeat([]byte{0x1c},32),{0x19,0xf5,0xff,0xff,0xff,0x7f},{0x16,0xff,0xff,0xff,0xff,0xff,0xff,0xff,0xff,0xff,0xff}} {
		if _,_,e:=compact(raw);e==nil{t.Fatal("unbounded compact structure accepted")}
	}
	if _,e:=guardRLE([]byte{0xff,0xff,0xff,0xff,0x7f},1,1,2,false);e==nil{t.Fatal("unbounded RLE run accepted")}
	if _,e:=boundedDecompress(0,[]byte("large"),1);e==nil{t.Fatal("declared expansion not enforced")}
	if e:=guardPlain([]byte{255,255,255,127},parquetColumn{physical:6},1,64);e==nil{t.Fatal("cell allocation not bounded")}
}

func FuzzUploadCSV(f *testing.F) {
	for _,s:=range []string{"id\n1\n","id\n\\N\n","id\n\"broken","Id,ID\n1,2"}{f.Add([]byte(s))}
	f.Fuzz(func(t *testing.T,b []byte){if len(b)==0||len(b)>8192{return};s:=parseSpec("csv",b,UploadColumn{"id","text",true});l:=config.DefaultUploads();l.MaxBytes=8192;l.MaxRows=64;l.MaxCells=64;l.MaxCellBytes=128;_,_=Parse(context.Background(),b,s,l,func(row []Cell)error{if len(row)!=1{t.Fatal("shape")};return nil})})
}
func FuzzUploadArchiveAndCompact(f *testing.F) {
	f.Add([]byte("PK\x03\x04"));f.Add([]byte{0x18,0xff,0xff,0xff,0xff,0x7f});f.Add([]byte("PAR1PAR1"))
	f.Fuzz(func(t *testing.T,b []byte){if len(b)==0||len(b)>32768{return};_,_,_=compact(b);l:=config.DefaultUploads();l.MaxBytes=32768;l.MaxExpandedBytes=32768;l.MaxArchiveEntries=16;l.MaxRows=64;l.MaxCells=256;l.MaxCellBytes=256
		for _,format:=range []string{"xlsx","parquet"}{s:=parseSpec(format,b,UploadColumn{"id","text",true});_,_=Parse(context.Background(),b,s,l,func([]Cell)error{return nil})}
	})
}
