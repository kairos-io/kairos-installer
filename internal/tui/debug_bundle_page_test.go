// internal/tui/debug_bundle_page_test.go (package tui)
package tui

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/kairos-io/kairos-installer/internal/debugbundle"
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

var _ = Describe("formatUSBMenu", func() {
	mounts := []debugbundle.RemovableMount{
		{Device: "/dev/sdb1", MountPoint: "/run/media/usb0"},
		{Device: "/dev/sdc1", MountPoint: "/run/media/usb1"},
	}

	It("lists every removable mount with its device and mount point", func() {
		out := formatUSBMenu(mounts, 0)
		Expect(out).To(ContainSubstring("/dev/sdb1"))
		Expect(out).To(ContainSubstring("/run/media/usb0"))
		Expect(out).To(ContainSubstring("/dev/sdc1"))
		Expect(out).To(ContainSubstring("/run/media/usb1"))
	})

	It("marks the row at the cursor with the selection indicator", func() {
		out := formatUSBMenu(mounts, 1)
		lines := []rune(out)
		_ = lines
		// The cursor row (second device) carries the ">" indicator; the first does not.
		Expect(out).To(MatchRegexp(`>\s.*/dev/sdc1`))
		Expect(out).ToNot(MatchRegexp(`>\s.*/dev/sdb1`))
	})
})
