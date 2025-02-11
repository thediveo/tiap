// Licensed under the Apache License, Version 2.0 (the "License"); you may not
// use this file except in compliance with the License. You may obtain a copy
// of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
// WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the
// License for the specific language governing permissions and limitations
// under the License.

package interpolate

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("lexing and parsing", func() {

	Context("identifiers", func() {

		It("parses an identifier", func() {
			Expect(parseName("_abc123DEF")).To(Equal("_abc123DEF"))
			Expect(parseName("_abc123def-foo")).To(Equal("_abc123def"))
			Expect(parseName("abc_123_def")).To(Equal("abc_123_def"))
		})

		It("rejects non-identifiers", func() {
			Expect(parseName("123")).To(BeZero())
			Expect(parseName("$")).To(BeZero())
		})

	})

	When("parsing into segments", func() {

		It("returns an empty string unmodified", func() {
			segments, err := parse("")
			Expect(err).NotTo(HaveOccurred())
			Expect(segments).To(BeEmpty())
		})

		It("returns a plain string unmodified", func() {
			segments, err := parse("foo {-} bar")
			Expect(err).NotTo(HaveOccurred())
			Expect(segments).To(HaveExactElements(PlainText("foo {-} bar")))
		})

		It("returns escaped delemiter", func() {
			segments, err := parse("foo$$bar")
			Expect(err).NotTo(HaveOccurred())
			Expect(segments).To(HaveExactElements(PlainText("foo$bar")))
		})

		It("parses an unbraced substitution", func() {
			segments, err := parse("foo$bar.baz")
			Expect(err).NotTo(HaveOccurred())
			Expect(segments).To(HaveExactElements(
				PlainText("foo"),
				Substitution{VariableName: "bar"},
				PlainText(".baz"),
			))
		})

		It("parses an unbraced substitution at end", func() {
			segments, err := parse("foo$bar")
			Expect(err).NotTo(HaveOccurred())
			Expect(segments).To(HaveExactElements(
				PlainText("foo"),
				Substitution{VariableName: "bar"},
			))
		})

		It("parses a braced substitution", func() {
			segments, err := parse("foo${bar}baz")
			Expect(err).NotTo(HaveOccurred())
			Expect(segments).To(HaveExactElements(
				PlainText("foo"),
				Substitution{VariableName: "bar"},
				PlainText("baz"),
			))
		})

		It("parses a braced substitution at end", func() {
			segments, err := parse("foo${bar}")
			Expect(err).NotTo(HaveOccurred())
			Expect(segments).To(HaveExactElements(
				PlainText("foo"),
				Substitution{VariableName: "bar"},
			))
		})

		DescribeTable("substitution operations",
			func(oper string) {
				segments, err := parse("foo${bar" + oper + "xxx}baz")
				Expect(err).NotTo(HaveOccurred())
				Expect(segments).To(HaveExactElements(
					PlainText("foo"),
					Substitution{
						VariableName: "bar",
						Operation:    oper,
						AltValue: []Segment{
							PlainText("xxx"),
						},
					},
					PlainText("baz"),
				))
			},
			Entry(nil, "?"),
			Entry(nil, "+"),
			Entry(nil, "-"),
			Entry(nil, ":?"),
			Entry(nil, ":+"),
			Entry(nil, ":-"),
		)

		It("fails on trailing delemiter", func() {
			segments, err := parse("foo$")
			Expect(err).To(HaveOccurred())
			Expect(segments).To(BeNil())
		})

		It("fails on unclosed braced substitution", func() {
			segments, err := parse("foo${")
			Expect(err).To(HaveOccurred())
			Expect(segments).To(BeNil())
		})

		It("fails on unclosed braced substitution", func() {
			segments, err := parse("foo${bar")
			Expect(err).To(HaveOccurred())
			Expect(segments).To(BeNil())
		})

		It("fails on unknown braced substitution operation", func() {
			segments, err := parse("foo${bar*abc}")
			Expect(err).To(HaveOccurred())
			Expect(segments).To(BeNil())
		})
		It("fails on unclosed braced substitution operation", func() {
			segments, err := parse("foo${bar?")
			Expect(err).To(HaveOccurred())
			Expect(segments).To(BeNil())
		})

		It("fails on unclosed braced substitution operation", func() {
			segments, err := parse("foo${bar:")
			Expect(err).To(HaveOccurred())
			Expect(segments).To(BeNil())
		})

		It("fails on unclosed braced substitution operation", func() {
			segments, err := parse("foo${bar:*")
			Expect(err).To(HaveOccurred())
			Expect(segments).To(BeNil())
		})

	})

	Context("segments", func() {

		Context("plain text", func() {

			It("renders text unmodified", func() {
				Expect(PlainText("foo$bar").Text(nil)).To(Equal("foo$bar"))
			})

		})

		Context("substitutions", func() {

			vars := map[string]string{
				"FOO": "bar",
				"BAR": "",
			}

			It("substitutes unbraced variables", func() {
				seg := Substitution{
					VariableName: "FOO",
				}
				Expect(seg.Text(vars)).To(Equal("bar"))
				Expect(seg.Text(nil)).To(Equal(""))
			})

			When("braced", func() {

				It("?", func() {
					seg := Substitution{
						Operation:    "?",
						VariableName: "FOO",
						AltValue:     Segments{PlainText("oh no!")},
					}
					Expect(seg.Text(vars)).To(Equal("bar"))
					Expect(seg.Text(nil)).Error().To(MatchError("oh no!"))
				})

				It(":?", func() {
					seg := Substitution{
						Operation:    ":?",
						VariableName: "FOO",
						AltValue:     Segments{PlainText("oh no!")},
					}
					Expect(seg.Text(vars)).To(Equal("bar"))
					Expect(seg.Text(nil)).Error().To(MatchError("oh no!"))

					segs := Segments{
						Substitution{
							Operation:    ":?",
							VariableName: "BAR",
							AltValue:     Segments{PlainText("oh no!")},
						},
					}
					Expect(segs.Text(vars)).Error().To(MatchError("oh no!"))
				})

				It("-", func() {
					seg := Substitution{
						Operation:    "-",
						VariableName: "FOO",
						AltValue:     Segments{PlainText("oh no!")},
					}
					Expect(seg.Text(vars)).To(Equal("bar"))
					Expect(seg.Text(nil)).To(Equal("oh no!"))
				})

				It(":-", func() {
					seg := Substitution{
						Operation:    ":-",
						VariableName: "FOO",
						AltValue:     Segments{PlainText("oh no!")},
					}
					Expect(seg.Text(vars)).To(Equal("bar"))
					Expect(seg.Text(nil)).To(Equal("oh no!"))

					seg = Substitution{
						Operation:    ":-",
						VariableName: "BAR",
						AltValue:     Segments{PlainText("oh no!")},
					}
					Expect(seg.Text(vars)).To(Equal("oh no!"))
				})

				It("+", func() {
					seg := Substitution{
						Operation:    "+",
						VariableName: "FOO",
						AltValue:     Segments{PlainText("oh no!")},
					}
					Expect(seg.Text(vars)).To(Equal("oh no!"))
					Expect(seg.Text(nil)).To(Equal(""))
				})

				It(":+", func() {
					seg := Substitution{
						Operation:    ":+",
						VariableName: "FOO",
						AltValue:     Segments{PlainText("oh no!")},
					}
					Expect(seg.Text(vars)).To(Equal("oh no!"))
					Expect(seg.Text(nil)).To(Equal(""))

					seg = Substitution{
						Operation:    ":+",
						VariableName: "BAR",
						AltValue:     Segments{PlainText("oh no!")},
					}
					Expect(seg.Text(vars)).To(Equal(""))
				})

			})

			DescribeTable("bad substitutions",
				func(oper string, missingisgood bool) {
					seg := Substitution{
						VariableName: "ZOO",
						Operation:    oper,
						AltValue: Segments{
							Substitution{
								Operation: "???",
							},
						},
					}
					if missingisgood {
						seg.VariableName = "FOO"
					}
					Expect(seg.Text(vars)).Error().To(HaveOccurred())
				},
				Entry(nil, "?", false),
				Entry(nil, "+", true),
				Entry(nil, "-", false),
				Entry(nil, ":?", false),
				Entry(nil, ":+", true),
				Entry(nil, ":-", false),
			)

		})

	})

	When("E2E", func() {

		vars := map[string]string{
			"FOO": "foo",
			"BAR": "bar",
		}

		It("interpolates", func() {
			segs, err := parse("This ${FOO} is ${BAR}")
			Expect(err).NotTo(HaveOccurred())
			Expect(segs.Text(vars)).To(Equal("This foo is bar"))
		})

		It("interpolates recursively", func() {
			segs, err := parse("What a ${FOO+${BAR}}")
			Expect(err).NotTo(HaveOccurred())
			Expect(segs.Text(vars)).To(Equal("What a bar"))
		})

	})

})
