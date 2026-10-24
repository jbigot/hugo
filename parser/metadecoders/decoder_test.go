// Copyright 2018 The Hugo Authors. All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package metadecoders

import (
	"encoding/json"
	stdxml "encoding/xml"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"testing"

	qt "github.com/frankban/quicktest"
	"github.com/gohugoio/hugo/common/herrors"
)

func TestUnmarshalXML(t *testing.T) {
	c := qt.New(t)

	xmlDoc := `<?xml version="1.0" encoding="utf-8" standalone="yes"?>
	<rss version="2.0"
		xmlns:atom="http://www.w3.org/2005/Atom">
		<channel>
			<title>Example feed</title>
			<link>https://example.com/</link>
			<description>Example feed</description>
			<generator>Hugo -- gohugo.io</generator>
			<language>en-us</language>
			<copyright>Example</copyright>
			<lastBuildDate>Fri, 08 Jan 2021 14:44:10 +0000</lastBuildDate>
			<atom:link href="https://example.com/feed.xml" rel="self" type="application/rss+xml"/>
			<item>
				<title>Example title</title>
				<link>https://example.com/2021/11/30/example-title/</link>
				<pubDate>Tue, 30 Nov 2021 15:00:00 +0000</pubDate>
				<guid>https://example.com/2021/11/30/example-title/</guid>
				<description>Example description</description>
			</item>
		</channel>
	</rss>`

	expect := map[string]any{
		"-atom": "http://www.w3.org/2005/Atom", "-version": "2.0",
		"channel": map[string]any{
			"copyright":   "Example",
			"description": "Example feed",
			"generator":   "Hugo -- gohugo.io",
			"item": map[string]any{
				"description": "Example description",
				"guid":        "https://example.com/2021/11/30/example-title/",
				"link":        "https://example.com/2021/11/30/example-title/",
				"pubDate":     "Tue, 30 Nov 2021 15:00:00 +0000",
				"title":       "Example title",
			},
			"language":      "en-us",
			"lastBuildDate": "Fri, 08 Jan 2021 14:44:10 +0000",
			"link": []any{"https://example.com/", map[string]any{
				"-href": "https://example.com/feed.xml",
				"-rel":  "self",
				"-type": "application/rss+xml",
			}},
			"title": "Example feed",
		},
	}

	d := Default

	m, err := d.Unmarshal([]byte(xmlDoc), XML)
	c.Assert(err, qt.IsNil)
	c.Assert(m, qt.DeepEquals, expect)
}

func xmlEl(tag string, attrs map[string]string, children ...any) map[string]any {
	if attrs == nil {
		attrs = map[string]string{}
	}
	if children == nil {
		children = []any{}
	}
	return map[string]any{"tag": tag, "attributes": attrs, "children": children}
}

// checkXMLDocument checks the invariants of the document XML layout.
func checkXMLDocument(c *qt.C, v any) {
	c.Helper()
	stack := []any{v}
	for len(stack) > 0 {
		el, ok := stack[len(stack)-1].(map[string]any)
		stack = stack[:len(stack)-1]
		c.Assert(ok, qt.IsTrue, qt.Commentf("element is not a map[string]any"))
		c.Assert(el, qt.HasLen, 3)
		_, ok = el["tag"].(string)
		c.Assert(ok, qt.IsTrue, qt.Commentf("tag: %T", el["tag"]))
		attrs, ok := el["attributes"].(map[string]string)
		c.Assert(ok && attrs != nil, qt.IsTrue, qt.Commentf("attributes: %T", el["attributes"]))
		children, ok := el["children"].([]any)
		c.Assert(ok && children != nil, qt.IsTrue, qt.Commentf("children: %T", el["children"]))
		prevString := false
		for _, child := range children {
			switch child := child.(type) {
			case string:
				c.Assert(child, qt.Not(qt.Equals), "")
				c.Assert(prevString, qt.IsFalse, qt.Commentf("consecutive strings"))
				prevString = true
			default:
				stack = append(stack, child)
				prevString = false
			}
		}
	}
}

const xmlIssuePara = `<para>Use <ref refid="namespacedemo_1a5d" kindref="member">make_widget</ref> to build one, it is <bold>fast</bold> and <computeroutput>cheap</computeroutput>.</para>`

var xmlDocumentCases = []struct {
	name, in string
	want     map[string]any
}{
	{"empty root, self-closing", `<r/>`, xmlEl("r", nil)},
	{"empty root, end tag", `<r></r>`, xmlEl("r", nil)},
	{"text-only root", `<r>t</r>`, xmlEl("r", nil, "t")},
	{"numeric text stays a string", `<r>1</r>`, xmlEl("r", nil, "1")},
	{"nesting", `<a><b><c/></b></a>`, xmlEl("a", nil, xmlEl("b", nil, xmlEl("c", nil)))},
	{"sibling order and repetition", `<r><b/><a/><b/></r>`, xmlEl("r", nil, xmlEl("b", nil), xmlEl("a", nil), xmlEl("b", nil))},
	{"issue example", xmlIssuePara, xmlEl("para", nil, "Use ", xmlEl("ref", map[string]string{"kindref": "member", "refid": "namespacedemo_1a5d"}, "make_widget"), " to build one, it is ", xmlEl("bold", nil, "fast"), " and ", xmlEl("computeroutput", nil, "cheap"), ".")},
	{"text around a child", `<r>a<b/>c</r>`, xmlEl("r", nil, "a", xmlEl("b", nil), "c")},
	{"nested mixed content", `<p>x<em>y<b>z</b></em>w</p>`, xmlEl("p", nil, "x", xmlEl("em", nil, "y", xmlEl("b", nil, "z")), "w")},
	{"indentation kept", "<r>\n  <a/>\n</r>", xmlEl("r", nil, "\n  ", xmlEl("a", nil), "\n")},
	{"whitespace-only element", `<r> </r>`, xmlEl("r", nil, " ")},
	{"tabs and newlines in text", "<r>\t\n </r>", xmlEl("r", nil, "\t\n ")},
	{"markup and whitespace outside the root dropped", "<?xml version=\"1.0\"?>\n<!-- c -->\n<?pi x?>\n<r/>\n<!-- c -->\n<?pi y?>\n", xmlEl("r", nil)},
	{"full XML declaration", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><r/>`, xmlEl("r", nil)},
	{"predefined entities", `<r>&lt;&gt;&amp;&apos;&quot;</r>`, xmlEl("r", nil, `<>&'"`)},
	{"character references", `<r>&#233;&#x263A;&#x1F600;</r>`, xmlEl("r", nil, "é☺😀")},
	{"literal non-ASCII text", `<r>é ☺ 😀</r>`, xmlEl("r", nil, "é ☺ 😀")},
	{"CRLF and CR in text", "<r>1\r\n2\r3</r>", xmlEl("r", nil, "1\n2\n3")},
	{"CDATA alone", `<r><![CDATA[<b>&amp;</b>]]></r>`, xmlEl("r", nil, "<b>&amp;</b>")},
	{"CDATA merged with text", `<r>a<![CDATA[b]]>c</r>`, xmlEl("r", nil, "abc")},
	{"adjacent CDATA sections", `<r><![CDATA[a]]><![CDATA[b]]></r>`, xmlEl("r", nil, "ab")},
	{"]]> split across CDATA", `<r><![CDATA[a]]]]><![CDATA[>b]]></r>`, xmlEl("r", nil, "a]]>b")},
	{"empty CDATA leaves no string", `<r><![CDATA[]]></r>`, xmlEl("r", nil)},
	{"comment merges text", `<r>a<!--c-->b</r>`, xmlEl("r", nil, "ab")},
	{"PI merges text", `<r>a<?pi x?>b</r>`, xmlEl("r", nil, "ab")},
	{"comment between elements", `<r><a/><!--c--><b/></r>`, xmlEl("r", nil, xmlEl("a", nil), xmlEl("b", nil))},
	{"lone comment leaves no string", `<r><!--c--></r>`, xmlEl("r", nil)},
	{"DOCTYPE dropped", "<!DOCTYPE r>\n<r/>", xmlEl("r", nil)},
	{"DOCTYPE with internal subset", `<!DOCTYPE r [<!ELEMENT r ANY><!ENTITY e "x">]><r/>`, xmlEl("r", nil)},
	{"DOCTYPE with external id", `<!DOCTYPE r SYSTEM "r.dtd"><r/>`, xmlEl("r", nil)},
	{"root attributes", `<r a="1" b="2"/>`, xmlEl("r", map[string]string{"a": "1", "b": "2"})},
	{"single-quoted value", `<r a='x "y"'/>`, xmlEl("r", map[string]string{"a": `x "y"`})},
	{"> in a value", `<r a="x > y"/>`, xmlEl("r", map[string]string{"a": "x > y"})},
	{"references in a value", `<r a="&lt;&amp;&#233;&#x263A;&quot;"/>`, xmlEl("r", map[string]string{"a": `<&é☺"`})},
	{"empty value", `<r a=""/>`, xmlEl("r", map[string]string{"a": ""})},
	{"spaces around =", `<r a = "1"/>`, xmlEl("r", map[string]string{"a": "1"})},
	{"multi-line start tag", "<r\n  a=\"1\"\n  b=\"2\"/>", xmlEl("r", map[string]string{"a": "1", "b": "2"})},
	{"newline reference in a value", `<r a="p&#10;q"/>`, xmlEl("r", map[string]string{"a": "p\nq"})},
	{"prefixes and xmlns kept", `<r xmlns="u" xmlns:x="v" xml:lang="fr"><x:e/></r>`, xmlEl("r", map[string]string{"xmlns": "u", "xmlns:x": "v", "xml:lang": "fr"}, xmlEl("x:e", nil))},
	{"undeclared element prefix", `<y:r/>`, xmlEl("y:r", nil)},
	{"undeclared attribute prefix", `<r x:a="1"/>`, xmlEl("r", map[string]string{"x:a": "1"})},
	{"same local name, other prefixes", `<r a="1" x:a="2" y:a="3"/>`, xmlEl("r", map[string]string{"a": "1", "x:a": "2", "y:a": "3"})},
	{"case kept", `<Mixed Case="yes"><mixed/></Mixed>`, xmlEl("Mixed", map[string]string{"Case": "yes"}, xmlEl("mixed", nil))},
	{"name characters", `<a-b.c_d1/>`, xmlEl("a-b.c_d1", nil)},
	{"non-ASCII names", `<é ü="1"/>`, xmlEl("é", map[string]string{"ü": "1"})},
	{"whitespace before > in an end tag", `<r></r >`, xmlEl("r", nil)},
	{"BOM at the start (string input)", "\ufeff<r/>", xmlEl("r", nil)},
}

// Malformed input accepted, as by the default layout.
var xmlDocumentLenientCases = []struct {
	name, in string
	want     map[string]any
}{
	{"text before the root ignored", `x<r/>`, xmlEl("r", nil)},
	{"content after the root ignored", "<r/>\nx<b/></r><a", xmlEl("r", nil)},
}

func TestUnmarshalXMLDocument(t *testing.T) {
	c := qt.New(t)

	for _, test := range slices.Concat(xmlDocumentCases, xmlDocumentLenientCases) {
		c.Run(test.name, func(c *qt.C) {
			var v any
			err := Default.unmarshalXMLDocument([]byte(test.in), &v)
			c.Assert(err, qt.IsNil)
			c.Assert(v, qt.DeepEquals, any(test.want))
			checkXMLDocument(c, v)
		})
	}
}

func TestUnmarshalXMLDocumentTargets(t *testing.T) {
	c := qt.New(t)

	var m map[string]any
	c.Assert(Default.unmarshalXMLDocument([]byte(`<e/>`), &m), qt.IsNil)
	c.Assert(m, qt.DeepEquals, xmlEl("e", nil))

	b, err := json.Marshal(m)
	c.Assert(err, qt.IsNil)
	c.Assert(string(b), qt.Equals, `{"attributes":{},"children":[],"tag":"e"}`)
}

func TestUnmarshalXMLDocumentDeep(t *testing.T) {
	c := qt.New(t)

	const depth = 10000
	in := strings.Repeat("<a>", depth) + "x" + strings.Repeat("</a>", depth)

	var v any
	c.Assert(Default.unmarshalXMLDocument([]byte(in), &v), qt.IsNil)
	checkXMLDocument(c, v)

	n := 0
	for {
		el, ok := v.(map[string]any)
		if !ok {
			break
		}
		n++
		children := el["children"].([]any)
		c.Assert(children, qt.HasLen, 1)
		v = children[0]
	}
	c.Assert(n, qt.Equals, depth)
	c.Assert(v, qt.Equals, "x")
}

var xmlDocumentErrorCases = []struct {
	name, in, want string
}{
	{"mismatched end tag", "<a>\n  <b></c>\n</a>", `"_stream.xml:2:10": unmarshal failed: element <b> closed by </c>`},
	{"mismatched prefix", `<x:a></y:a>`, `"_stream.xml:1:12": unmarshal failed: element <x:a> closed by </y:a>`},
	{"end tag without an open element", `</a>`, `"_stream.xml:1:5": unmarshal failed: unexpected end element </a>`},
	{"end tag closing an inner element early", `<r><a></r>`, `"_stream.xml:1:11": unmarshal failed: element <a> closed by </r>`},
	{"unclosed element", "<a>\n<b></b>\n", `"_stream.xml:3:1": unmarshal failed: unclosed element <a>`},
	{"innermost unclosed element named", `<a><b>`, `"_stream.xml:1:7": unmarshal failed: unclosed element <b>`},
	{"empty", "", `"_stream.xml:1:1": unmarshal failed: no root element`},
	{"text only", `x`, `"_stream.xml:1:2": unmarshal failed: no root element`},
	{"whitespace only", "  \n", `"_stream.xml:2:1": unmarshal failed: no root element`},
	{"comment only", `<!-- c -->`, `"_stream.xml:1:11": unmarshal failed: no root element`},
	{"PI only", `<?pi x?>`, `"_stream.xml:1:9": unmarshal failed: no root element`},
	{"DOCTYPE only", `<!DOCTYPE r>`, `"_stream.xml:1:13": unmarshal failed: no root element`},
	{"error from encoding/xml", "<r>\n &foo;</r>", `unmarshal failed: XML syntax error on line 2: invalid character entity &foo;`},
}

// Errors from encoding/xml: only the line of their position is checked.
var xmlDocumentSyntaxErrorCases = []struct {
	name, in string
	line     int
}{
	{"unknown entity", "<r>\n &foo;</r>", 2},
	{"HTML entity", `<r>&nbsp;</r>`, 1},
	{"entity from the internal subset", `<!DOCTYPE r [<!ENTITY foo "x">]><r>&foo;</r>`, 1},
	{"bare ampersand", `<r>a & b</r>`, 1},
	{"ampersand in a value", `<r a="&"/>`, 1},
	{"bare less-than", `<r>a < b</r>`, 1},
	{"less-than in a value", `<r a="<"/>`, 1},
	{"]]> in text", `<r>]]></r>`, 1},
	{"unterminated value", `<r a="x/>`, 1},
	{"attribute without a value", `<r a/>`, 1},
	{"unquoted value", `<r a=1/>`, 1},
	{"unclosed comment", `<r><!-- x</r>`, 1},
	{"-- in a comment", `<r><!-- a -- b --></r>`, 1},
	{"unclosed CDATA", `<r><![CDATA[x</r>`, 1},
	{"invalid name", `<1a/>`, 1},
	{"empty name", `<></>`, 1},
	{"attribute in an end tag", `<r></r a="1">`, 1},
	{"illegal character", "<r>\x01</r>", 1},
	{"reference to an illegal character", `<r>&#1;</r>`, 1},
	{"reference out of range", `<r>&#x110000;</r>`, 1},
	{"invalid UTF-8", "<r>\xff</r>", 1},
	{"non-UTF-8 encoding", `<?xml version="1.0" encoding="ISO-8859-1"?><r/>`, 1},
}

func xmlRawTokenError(in string) error {
	dec := stdxml.NewDecoder(strings.NewReader(in))
	for {
		if _, err := dec.RawToken(); err != nil {
			return err
		}
	}
}

func TestUnmarshalXMLDocumentErrors(t *testing.T) {
	c := qt.New(t)

	for _, test := range xmlDocumentErrorCases {
		c.Run(test.name, func(c *qt.C) {
			var v any
			err := Default.unmarshalXMLDocument([]byte(test.in), &v)
			c.Assert(err, qt.IsNotNil)
			c.Assert(err.Error(), qt.Contains, test.want)
		})
	}

	for _, test := range xmlDocumentSyntaxErrorCases {
		c.Run(test.name, func(c *qt.C) {
			var v any
			err := Default.unmarshalXMLDocument([]byte(test.in), &v)
			c.Assert(err, qt.ErrorMatches, fmt.Sprintf(`"_stream\.xml:%d:\d+": unmarshal failed: %s`, test.line, regexp.QuoteMeta(xmlRawTokenError(test.in).Error())))
		})
	}

	in := `<?xml version="1.0" encoding="ISO-8859-1"?><r/>`
	_, err := Default.Unmarshal([]byte(in), XML)
	c.Assert(err, qt.ErrorMatches, `.* `+regexp.QuoteMeta(xmlRawTokenError(in).Error()))
}

func TestUnmarshalXMLDocumentNoRoot(t *testing.T) {
	c := qt.New(t)

	d := Decoder{Delimiter: ',', TargetType: "document"}
	for _, in := range []string{"  \n", "<!-- c -->"} {
		_, err := d.Unmarshal([]byte(in), XML)
		c.Assert(err, qt.ErrorMatches, `.*no root element`, qt.Commentf("%q", in))
	}
}

func TestUnmarshalTargetType(t *testing.T) {
	c := qt.New(t)

	c.Check(Default.TargetType, qt.Equals, "")

	const (
		x   = `<r a="1">t</r>`
		csv = "a,b\n1,2"
	)
	xmlTargetTypeErr := "invalid targetType: expected either data or document, received "
	csvTargetTypeErr := "invalid targetType: expected either slice or map, received "

	for _, test := range []struct {
		format      Format
		targetTypes []string
		data        []string
		want        any
		err         string // completed with the targetType
	}{
		{XML, []string{"", "data"}, []string{x}, map[string]any{"-a": "1", "#text": "t"}, ""},
		{XML, []string{"document"}, []string{x}, xmlEl("r", map[string]string{"a": "1"}, "t"), ""},
		{XML, []string{"", "data", "document"}, []string{""}, map[string]any{}, ""},
		{XML, []string{"slice", "map", "foo"}, []string{x, ""}, nil, xmlTargetTypeErr},
		{CSV, []string{"", "slice"}, []string{csv}, [][]string{{"a", "b"}, {"1", "2"}}, ""},
		{CSV, []string{"", "slice"}, []string{""}, [][]string{}, ""},
		{CSV, []string{"map"}, []string{csv}, []map[string]string{{"a": "1", "b": "2"}}, ""},
		{CSV, []string{"map"}, []string{""}, map[string]any{}, ""},
		{CSV, []string{"data", "document", "foo"}, []string{csv, ""}, nil, csvTargetTypeErr},
		{JSON, []string{"document", "foo"}, []string{`{"a":1}`}, map[string]any{"a": float64(1)}, ""},
	} {
		for _, tt := range test.targetTypes {
			for _, data := range test.data {
				c.Run(fmt.Sprintf("%s/%q/%q", test.format, tt, data), func(c *qt.C) {
					d := Decoder{Delimiter: ',', TargetType: tt}
					v, err := d.Unmarshal([]byte(data), test.format)
					if test.err != "" {
						c.Assert(err, qt.IsNotNil)
						c.Assert(err.Error(), qt.Contains, test.err+tt)
						return
					}
					c.Assert(err, qt.IsNil)
					c.Assert(v, qt.DeepEquals, test.want)
				})
			}
		}
	}
}

func TestUnmarshalXMLDocumentFileError(t *testing.T) {
	c := qt.New(t)

	d := Decoder{Delimiter: ',', TargetType: "document"}
	_, err := d.Unmarshal([]byte("<a>\n  <b></c>\n</a>"), XML)
	c.Assert(err, qt.IsNotNil)
	fe := herrors.UnwrapFileError(err)
	c.Assert(fe, qt.IsNotNil)
	c.Assert(fe.Position().LineNumber, qt.Equals, 2)
	c.Assert(fe.Position().ColumnNumber, qt.Equals, 10)
}

// TestUnmarshalXMLDefaultLayout pins the output of the default XML layout.
func TestUnmarshalXMLDefaultLayout(t *testing.T) {
	c := qt.New(t)

	for _, test := range []struct {
		in   string
		want any // or an error substring, as a string
	}{
		{xmlIssuePara, map[string]any{
			"#text": "Use", "bold": "fast", "computeroutput": "cheap",
			"ref": map[string]any{"#text": "make_widget", "-kindref": "member", "-refid": "namespacedemo_1a5d"},
		}},
		{`<r><b>1</b><a/><b>2</b></r>`, map[string]any{"a": "", "b": []any{"1", "2"}}},
		{`<r xmlns:x="u"><x:a> t </x:a></r>`, map[string]any{"-x": "u", "a": "t"}},
		{`<r>t</r>`, "must be a map/object"},
	} {
		c.Run(test.in, func(c *qt.C) {
			v, err := Default.Unmarshal([]byte(test.in), XML)
			if s, ok := test.want.(string); ok {
				c.Assert(err, qt.IsNotNil)
				c.Assert(err.Error(), qt.Contains, s)
				return
			}
			c.Assert(err, qt.IsNil)
			c.Assert(v, qt.DeepEquals, test.want)
		})
	}
}

func TestUnmarshalToMap(t *testing.T) {
	c := qt.New(t)

	expect := map[string]any{"a": "b"}

	d := Default

	for i, test := range []struct {
		data   string
		format Format
		expect any
	}{
		{`a = "b"`, TOML, expect},
		{`a: "b"`, YAML, expect},
		// Make sure we get all string keys, even for YAML
		{"a: Easy!\nb:\n  c: 2\n  d: [3, 4]", YAML, map[string]any{"a": "Easy!", "b": map[string]any{"c": uint64(2), "d": []any{uint64(3), uint64(4)}}}},
		{"a:\n  true: 1\n  false: 2", YAML, map[string]any{"a": map[string]any{"true": uint64(1), "false": uint64(2)}}},
		{`{ "a": "b" }`, JSON, expect},
		{`<root><a>b</a></root>`, XML, expect},
		{`#+a: b`, ORG, expect},
		// errors
		{`a = b`, TOML, false},
		{`a,b,c`, CSV, false}, // Use Unmarshal for CSV
		{`<root>just a string</root>`, XML, false},
	} {
		msg := qt.Commentf("%d: %s", i, test.format)
		m, err := d.UnmarshalToMap([]byte(test.data), test.format)
		if b, ok := test.expect.(bool); ok && !b {
			c.Assert(err, qt.Not(qt.IsNil), msg)
		} else {
			c.Assert(err, qt.IsNil, msg)
			c.Assert(m, qt.DeepEquals, test.expect, msg)
		}
	}
}

func TestUnmarshalToInterface(t *testing.T) {
	c := qt.New(t)

	expect := map[string]any{"a": "b"}

	d := Default

	for i, test := range []struct {
		data   []byte
		format Format
		expect any
	}{
		{[]byte(`[ "Brecker", "Blake", "Redman" ]`), JSON, []any{"Brecker", "Blake", "Redman"}},
		{[]byte(`{ "a": "b" }`), JSON, expect},
		{[]byte(``), JSON, map[string]any{}},
		{[]byte(nil), JSON, map[string]any{}},
		{[]byte(`#+a: b`), ORG, expect},
		{[]byte("#+a: foo bar\n#+a: baz"), ORG, map[string]any{"a": []string{string("foo bar"), string("baz")}}},
		{[]byte(`#+DATE: <2020-06-26 Fri>`), ORG, map[string]any{"date": "2020-06-26"}},
		{[]byte(`#+LASTMOD: <2020-06-26 Fri>`), ORG, map[string]any{"lastmod": "2020-06-26"}},
		{[]byte(`#+FILETAGS: :work:`), ORG, map[string]any{"filetags": []string{"work"}}},
		{[]byte(`#+FILETAGS: :work:fun:`), ORG, map[string]any{"filetags": []string{"work", "fun"}}},
		{[]byte(`#+PUBLISHDATE: <2020-06-26 Fri>`), ORG, map[string]any{"publishdate": "2020-06-26"}},
		{[]byte(`#+EXPIRYDATE: <2020-06-26 Fri>`), ORG, map[string]any{"expirydate": "2020-06-26"}},
		{[]byte(`a = "b"`), TOML, expect},
		{[]byte(`a: "b"`), YAML, expect},
		{[]byte(`<root><a>b</a></root>`), XML, expect},
		{[]byte(`a,b,c`), CSV, [][]string{{"a", "b", "c"}}},
		{[]byte("a: Easy!\nb:\n  c: 2\n  d: [3, 4]"), YAML, map[string]any{"a": "Easy!", "b": map[string]any{"c": uint64(2), "d": []any{uint64(3), uint64(4)}}}},
		// errors
		{[]byte(`a = "`), TOML, false},
	} {
		msg := qt.Commentf("%d: %s", i, test.format)
		m, err := d.Unmarshal(test.data, test.format)
		if b, ok := test.expect.(bool); ok && !b {
			c.Assert(err, qt.Not(qt.IsNil), msg)
		} else {
			c.Assert(err, qt.IsNil, msg)
			c.Assert(m, qt.DeepEquals, test.expect, msg)
		}

	}
}

func TestUnmarshalStringTo(t *testing.T) {
	c := qt.New(t)

	d := Default

	expectMap := map[string]any{"a": "b"}

	for i, test := range []struct {
		data   string
		to     any
		expect any
	}{
		{"a string", "string", "a string"},
		{`{ "a": "b" }`, make(map[string]any), expectMap},
		{"32", int64(1234), int64(32)},
		{"32", int(1234), int(32)},
		{"3.14159", float64(1), float64(3.14159)},
		{"[3,7,9]", []any{}, []any{uint64(3), uint64(7), uint64(9)}},
		{"[3.1,7.2,9.3]", []any{}, []any{3.1, 7.2, 9.3}},
	} {
		msg := qt.Commentf("%d: %T", i, test.to)
		m, err := d.UnmarshalStringTo(test.data, test.to)
		if b, ok := test.expect.(bool); ok && !b {
			c.Assert(err, qt.Not(qt.IsNil), msg)
		} else {
			c.Assert(err, qt.IsNil, msg)
			c.Assert(m, qt.DeepEquals, test.expect, msg)
		}

	}
}

func TestCalculateAliasLimit(t *testing.T) {
	c := qt.New(t)

	const kb = 1024

	c.Assert(calculateCollectionAliasLimit(0), qt.Equals, 100)
	c.Assert(calculateCollectionAliasLimit(500), qt.Equals, 100)
	c.Assert(calculateCollectionAliasLimit(1*kb), qt.Equals, 100)
	c.Assert(calculateCollectionAliasLimit(2*kb), qt.Equals, 5000)
	c.Assert(calculateCollectionAliasLimit(8*kb), qt.Equals, 5000)
	c.Assert(calculateCollectionAliasLimit(12*kb), qt.Equals, 10000)
	c.Assert(calculateCollectionAliasLimit(10000*kb), qt.Equals, 10000)
}

func BenchmarkDecodeYAMLToMap(b *testing.B) {
	d := Default

	data := []byte(`
a:
  v1: 32
  v2: 43
  v3: "foo"
b:
  - a
  - b
c: "d"

`)

	for b.Loop() {
		_, err := d.UnmarshalToMap(data, YAML)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkUnmarshalBillionLaughs(b *testing.B) {
	yamlBillionLaughs := []byte(`
a: &a [_, _, _, _, _, _, _, _, _, _, _, _, _, _, _]
b: &b [*a, *a, *a, *a, *a, *a, *a, *a, *a, *a]
c: &c [*b, *b, *b, *b, *b, *b, *b, *b, *b, *b]
d: &d [*c, *c, *c, *c, *c, *c, *c, *c, *c, *c]
e: &e [*d, *d, *d, *d, *d, *d, *d, *d, *d, *d]
f: &f [*e, *e, *e, *e, *e, *e, *e, *e, *e, *e]
g: &g [*f, *f, *f, *f, *f, *f, *f, *f, *f, *f]
h: &h [*g, *g, *g, *g, *g, *g, *g, *g, *g, *g]
i: &i [*h, *h, *h, *h, *h, *h, *h, *h, *h, *h]
`)

	yamlFrontMatter := []byte(`
title: mysect
tags: [tag1, tag2]
params:
  color: blue
`)

	yamlTests := []struct {
		Title                      string
		Content                    []byte
		IsExpectedToFailValidation bool
	}{
		{"Billion Laughs", yamlBillionLaughs, true},
		{"YAML Front Matter", yamlFrontMatter, false},
	}

	for _, tt := range yamlTests {

		b.Run(tt.Title+" no validation", func(b *testing.B) {
			for b.Loop() {
				var v any
				if err := unmarshalYamlNoValidation(tt.Content, &v); err != nil {
					b.Fatal(err)
				}
			}
		})

		b.Run(tt.Title+" with validation", func(b *testing.B) {
			for b.Loop() {
				var v any
				err := UnmarshalYaml(tt.Content, &v)
				if tt.IsExpectedToFailValidation {
					if err == nil {
						b.Fatal("expected to fail validation but did not")
					}
				} else {
					if err != nil {
						b.Fatal(err)
					}
				}
			}
		})
	}
}
