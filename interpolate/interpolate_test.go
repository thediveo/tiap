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
	"gopkg.in/yaml.v3"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("interpolating", func() {

	It("interpolates YAML", func() {
		var yammel map[string]any
		Expect(yaml.Unmarshal([]byte(`
foo:
  fool: 42
  bar:
    - baz=***$FOO***
`),
			&yammel)).To(Succeed())
		Expect(Variables(yammel, map[string]string{
			"FOO": "---",
		})).To(
			HaveKeyWithValue("foo",
				HaveKeyWithValue("bar",
					ConsistOf("baz=***---***"))))
	})

	It("returns interpolation errors", func() {
		var yammel map[string]any
		Expect(yaml.Unmarshal([]byte(`
foo:
  fool: 42
  bar:
    - baz=${FOO
`),
			&yammel)).To(Succeed())
		Expect(Variables(yammel, map[string]string{
			"FOO": "---",
		})).Error().To(MatchError("error in 'bar[0]': unterminated ${"))

	})

	It("returns a value as is if it isn't a string or something we can explore further", func() {
		Expect(recursively(42, "", nil)).To(Equal(42))
	})

	When("given a string", func() {

		It("interpolates", func() {
			Expect(interpolateString("***$FOO***", "", map[string]string{
				"FOO": "---",
			})).To(Equal("***---***"))
		})

		It("reports interpolation errors", func() {
			Expect(interpolateString("${FOO", "name", nil)).Error().To(
				MatchError("error in 'name': unterminated ${"))
		})

	})

	When("given a mapping", func() {

		It("interpolates", func() {
			Expect(interpolateMapping(
				map[string]any{
					"foo": "***$FOO***",
				}, "", map[string]string{
					"FOO": "---",
				})).To(HaveKeyWithValue("foo", Equal("***---***")))
		})

		It("reports interpolation errors", func() {
			Expect(interpolateMapping(
				map[string]any{
					"foo": "${FOO",
				}, "mapping", nil)).Error().To(
				MatchError("error in 'mapping.foo': unterminated ${"))
		})

	})

	When("given a sequence", func() {

		It("interpolates", func() {
			Expect(interpolateSequence(
				[]any{
					"***$FOO***",
				}, "", map[string]string{
					"FOO": "---",
				})).To(ConsistOf("***---***"))
		})

		It("reports interpolation errors", func() {
			Expect(interpolateSequence(
				[]any{
					"",
					"${FOO",
				}, "sequence", nil)).Error().To(
				MatchError("error in 'sequence[1]': unterminated ${"))
		})

	})

})
