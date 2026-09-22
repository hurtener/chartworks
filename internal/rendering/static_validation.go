package rendering

import (
	"encoding/xml"
	"io"
	"strings"

	nethtml "golang.org/x/net/html"
)

var safeElements = map[string]bool{
	"html": true, "head": true, "meta": true, "style": true, "body": true, "main": true,
	"section": true, "article": true, "h1": true, "h2": true, "h3": true, "p": true,
	"table": true, "thead": true, "tbody": true, "tfoot": true, "tr": true, "th": true, "td": true, "caption": true,
	"div": true, "span": true, "ul": true, "ol": true, "li": true, "strong": true, "em": true, "small": true, "br": true,
	"svg": true, "g": true, "path": true, "rect": true, "text": true, "polygon": true, "polyline": true, "circle": true, "title": true,
}
var safeAttributes = map[string]bool{
	"class": true, "style": true, "role": true, "width": true, "height": true, "viewbox": true,
	"fill": true, "stroke": true, "d": true, "x": true, "y": true, "cx": true, "cy": true, "r": true, "points": true,
	"fill-opacity": true, "stroke-width": true, "font-weight": true, "xmlns": true, "colspan": true, "rowspan": true,
	"charset": true, "http-equiv": true, "content": true, "name": true,
}

func safeStatic(format, content string) bool {
	switch format {
	case "html":
		return validateHTML(content)
	case "svg":
		return validateSVG(content)
	default:
		return false
	}
}

func safeCSS(value string) bool {
	v := strings.ToLower(value)
	for _, bad := range []string{"url", "@import", "expression", "behavior", "javascript", "\\", "/*", "*/"} {
		if strings.Contains(v, bad) {
			return false
		}
	}
	return true
}
func safeAttribute(element, name, value string) bool {
	name = strings.ToLower(name)
	if strings.HasPrefix(name, "data-") || strings.HasPrefix(name, "aria-") {
		return true
	}
	if !safeAttributes[name] {
		return false
	}
	if name == "style" && !safeCSS(value) {
		return false
	}
	if name == "xmlns" && value != "http://www.w3.org/2000/svg" {
		return false
	}
	if name == "http-equiv" && strings.ToLower(value) != "content-security-policy" {
		return false
	}
	if name == "content" && element == "meta" && !safeCSS(value) {
		return false
	}
	return true
}
func validateHTML(content string) bool {
	if !strings.HasPrefix(strings.ToLower(content), "<!doctype html>") {
		return false
	}
	z := nethtml.NewTokenizer(strings.NewReader(content))
	doctype, htmlElement, styleDepth := false, false, 0
	for {
		typeOfToken := z.Next()
		switch typeOfToken {
		case nethtml.ErrorToken:
			return z.Err() == io.EOF && doctype && htmlElement && styleDepth == 0
		case nethtml.DoctypeToken:
			if doctype || !strings.EqualFold(z.Token().Data, "html") {
				return false
			}
			doctype = true
		case nethtml.CommentToken:
			return false
		case nethtml.StartTagToken, nethtml.SelfClosingTagToken:
			token := z.Token()
			element := strings.ToLower(token.Data)
			if !safeElements[element] {
				return false
			}
			if element == "html" {
				htmlElement = true
			}
			if element == "style" && typeOfToken == nethtml.StartTagToken {
				styleDepth++
			}
			for _, attribute := range token.Attr {
				if attribute.Namespace != "" || !safeAttribute(element, attribute.Key, attribute.Val) {
					return false
				}
			}
		case nethtml.EndTagToken:
			element := strings.ToLower(z.Token().Data)
			if !safeElements[element] {
				return false
			}
			if element == "style" {
				styleDepth--
				if styleDepth < 0 {
					return false
				}
			}
		case nethtml.TextToken:
			if styleDepth > 0 && !safeCSS(string(z.Text())) {
				return false
			}
		}
	}
}
func validateSVG(content string) bool {
	if !strings.HasPrefix(strings.ToLower(content), "<svg ") || !strings.HasSuffix(strings.ToLower(content), "</svg>") {
		return false
	}
	d := xml.NewDecoder(strings.NewReader(content))
	depth := 0
	for {
		token, err := d.Token()
		if err == io.EOF {
			return depth == 0
		}
		if err != nil {
			return false
		}
		switch v := token.(type) {
		case xml.StartElement:
			e := strings.ToLower(v.Name.Local)
			if !safeElements[e] || e == "html" || e == "head" || e == "meta" || e == "style" || e == "body" || e == "main" || e == "section" || e == "article" || e == "table" {
				return false
			}
			depth++
			for _, a := range v.Attr {
				name := strings.ToLower(a.Name.Local)
				if a.Name.Space == "xmlns" {
					name = "xmlns"
				}
				if !safeAttribute(e, name, a.Value) {
					return false
				}
			}
		case xml.EndElement:
			depth--
			if depth < 0 {
				return false
			}
		case xml.Directive, xml.ProcInst, xml.Comment:
			return false
		}
	}
}
