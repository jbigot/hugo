// Copyright 2026 The Hugo Authors. All rights reserved.
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
	stdxml "encoding/xml"
	"maps"
	"slices"
	"strings"
	"testing"

	qt "github.com/frankban/quicktest"
)

// writeXMLDocument serializes el. EscapeText also escapes tabs and newlines,
// so that decoding the result gives the same tree.
func writeXMLDocument(sb *strings.Builder, el map[string]any) {
	tag := el["tag"].(string)
	attrs := el["attributes"].(map[string]string)
	sb.WriteString("<" + tag)
	for _, name := range slices.Sorted(maps.Keys(attrs)) {
		sb.WriteString(" " + name + `="`)
		stdxml.EscapeText(sb, []byte(attrs[name]))
		sb.WriteString(`"`)
	}
	sb.WriteString(">")
	for _, child := range el["children"].([]any) {
		switch child := child.(type) {
		case string:
			stdxml.EscapeText(sb, []byte(child))
		case map[string]any:
			writeXMLDocument(sb, child)
		}
	}
	sb.WriteString("</" + tag + ">")
}

func assertXMLDocumentRoundTrip(c *qt.C, v any) {
	c.Helper()
	var sb strings.Builder
	writeXMLDocument(&sb, v.(map[string]any))
	var w any
	c.Assert(Default.unmarshalXMLDocument([]byte(sb.String()), &w), qt.IsNil, qt.Commentf("%s", sb.String()))
	c.Assert(w, qt.DeepEquals, v)
}

func TestUnmarshalXMLDocumentRoundTrip(t *testing.T) {
	c := qt.New(t)

	for _, test := range slices.Concat(xmlDocumentCases, xmlDocumentLenientCases) {
		c.Run(test.name, func(c *qt.C) {
			var v any
			c.Assert(Default.unmarshalXMLDocument([]byte(test.in), &v), qt.IsNil)
			assertXMLDocumentRoundTrip(c, v)
		})
	}
}

func FuzzUnmarshalXMLDocument(f *testing.F) {
	for _, test := range slices.Concat(xmlDocumentCases, xmlDocumentLenientCases) {
		f.Add([]byte(test.in))
	}
	for _, test := range xmlDocumentErrorCases {
		f.Add([]byte(test.in))
	}
	for _, test := range xmlDocumentSyntaxErrorCases {
		f.Add([]byte(test.in))
	}

	f.Fuzz(func(t *testing.T, b []byte) {
		var v any
		if err := Default.unmarshalXMLDocument(b, &v); err != nil {
			return
		}
		c := qt.New(t)
		checkXMLDocument(c, v)
		assertXMLDocumentRoundTrip(c, v)
	})
}
