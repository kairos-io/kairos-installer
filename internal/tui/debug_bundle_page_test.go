// internal/tui/debug_bundle_page_test.go (package tui)
package tui

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("formatRetrievalText", func() {
	It("lists every URL and the local path", func() {
		out := formatRetrievalText("/run/kairos/b.tar.gz", []string{
			"http://10.0.0.5:8080/tok/b.tar.gz",
			"http://192.168.1.9:8080/tok/b.tar.gz",
		})
		Expect(out).To(ContainSubstring("10.0.0.5"))
		Expect(out).To(ContainSubstring("192.168.1.9"))
		Expect(out).To(ContainSubstring("/run/kairos/b.tar.gz"))
	})

	It("explains when no network URLs are available", func() {
		out := formatRetrievalText("/run/kairos/b.tar.gz", nil)
		Expect(out).To(ContainSubstring("no usable")) // no-IP message
		Expect(out).To(ContainSubstring("/run/kairos/b.tar.gz"))
	})
})
