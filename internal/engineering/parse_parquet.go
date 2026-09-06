package engineering

import (
	"bytes"
	"encoding/hex"
	"errors"
	"io"
	"math/big"
	"strconv"
	"strings"
	"time"

	"github.com/parquet-go/parquet-go"
)

func (p *parser) parquet(raw []byte) error {
	columns,err:=p.parquetPreflight(raw); if err!=nil { return err }
	file,err:=parquet.OpenFile(bytes.NewReader(raw),int64(len(raw)),&parquet.FileConfig{SkipPageIndex:true,SkipBloomFilters:true,ReadBufferSize:4096,ReadMode:parquet.ReadModeSync})
	if err!=nil { return ErrFormat }
	for _,group:=range file.RowGroups() {
		rows:=group.Rows(); buffer:=make([]parquet.Row,1)
		for {
			if err=p.ctx.Err(); err!=nil { _=rows.Close(); return err }
			n,readErr:=rows.ReadRows(buffer)
			if n<0 || n>1 || n==0 && readErr==nil { _=rows.Close(); return ErrFormat }
			if n==1 {
				row:=buffer[0]
				if len(row)!=len(columns) { _=rows.Close(); return ErrFormat }
				cells:=make([]Cell,len(columns))
				for i,v:=range row {
					if v.Column()!=i { _=rows.Close(); return ErrFormat }
					cells[i],err=parquetCell(v,columns[i],p.spec.Columns[i])
					if err!=nil { _=rows.Close(); return &ParseError{Code:"parquet_type",Row:p.receipt.Rows+2,Column:i+1} }
				}
				if err=p.row(cells); err!=nil { _=rows.Close(); return err }
			}
			if readErr!=nil {
				closeErr:=rows.Close()
				if !errors.Is(readErr,io.EOF) || closeErr!=nil { return ErrFormat }
				break
			}
		}
	}
	if int64(p.receipt.Rows)!=file.NumRows() { return ErrFormat }; return nil
}

func parquetCell(v parquet.Value, c parquetColumn, declared UploadColumn) (Cell,error) {
	if v.IsNull() { return Cell{Null:true},nil }
	if int(v.Kind())!=c.physical { return Cell{},ErrFormat }
	var s string
	if c.decimal {
		var n *big.Int
		switch c.physical {
		case 1:n=big.NewInt(int64(v.Int32()))
		case 2:n=big.NewInt(v.Int64())
		case 6,7:
			raw:=v.ByteArray(); if len(raw)<1 || len(raw)>32 { return Cell{},ErrFormat }
			n=new(big.Int).SetBytes(raw)
			if raw[0]&128!=0 { n.Sub(n,new(big.Int).Lsh(big.NewInt(1),uint(len(raw)*8))) }
		default:return Cell{},ErrFormat
		}
		s=scaledInteger(n,c.scale)
	} else if c.date {
		n:=int64(v.Int32()); date:=time.Unix(n*86400,0).UTC()
		if date.Year()<0 || date.Year()>9999 { return Cell{},ErrFormat }; s=date.Format("2006-01-02")
	} else if c.timestamp!=0 {
		n:=v.Int64(); unit:=int64(c.timestamp); stamp:=time.Unix(n/unit,(n%unit)*(1000000000/unit)).UTC()
		if stamp.Year()<0 || stamp.Year()>9999 { return Cell{},ErrFormat }; s=stamp.Format(time.RFC3339Nano)
	} else {
		switch c.physical {
		case 0:
			if declared.Type!="boolean" { return Cell{},ErrFormat }; s=strconv.FormatBool(v.Boolean())
		case 1:
			if declared.Type!="integer" && declared.Type!="decimal" { return Cell{},ErrFormat }; s=strconv.FormatInt(int64(v.Int32()),10)
		case 2:
			if declared.Type!="integer" && declared.Type!="decimal" { return Cell{},ErrFormat }; s=strconv.FormatInt(v.Int64(),10)
		case 4:
			if declared.Type!="number" { return Cell{},ErrFormat }; s=strconv.FormatFloat(float64(v.Float()),'g',-1,32)
		case 5:
			if declared.Type!="number" { return Cell{},ErrFormat }; s=strconv.FormatFloat(v.Double(),'g',-1,64)
		case 6,7:
			if declared.Type=="binary" { s=hex.EncodeToString(v.ByteArray()) } else if declared.Type=="text" || declared.Type=="json" || declared.Type=="decimal" || declared.Type=="date" || declared.Type=="timestamp" { s=string(v.ByteArray()) } else { return Cell{},ErrFormat }
		default:return Cell{},ErrFormat
		}
	}
	return Cell{Text:s},nil
}
func scaledInteger(n *big.Int, scale int) string {
	sign:=""; if n.Sign()<0 { sign="-" }
	s:=new(big.Int).Abs(n).String()
	if scale==0 { return sign+s }
	if len(s)<=scale { s=strings.Repeat("0",scale-len(s)+1)+s }
	return sign+s[:len(s)-scale]+"."+s[len(s)-scale:]
}
