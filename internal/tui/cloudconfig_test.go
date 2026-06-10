package tui

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("RenderCloudConfig", func() {
	It("renders a user stage with ssh keys at the network stage", func() {
		m := &Model{
			disk:         "/dev/sda",
			username:     "kairos",
			password:     "hashhash",
			sshKeys:      []string{"ssh-ed25519 AAAA"},
			finishAction: "reboot",
		}
		out, err := RenderCloudConfig(m)
		Expect(err).ToNot(HaveOccurred())
		Expect(out).To(HavePrefix("#cloud-config\n"))
		Expect(out).To(ContainSubstring("network:"))
		Expect(out).To(ContainSubstring("kairos"))
		Expect(out).To(ContainSubstring("ssh-ed25519 AAAA"))
		Expect(out).To(ContainSubstring("reboot: true"))
	})

	It("sets no_users and no user stage when no username is given", func() {
		m := &Model{disk: "/dev/sda", finishAction: "nothing"}
		out, err := RenderCloudConfig(m)
		Expect(err).ToNot(HaveOccurred())
		Expect(out).To(ContainSubstring("nousers: true"))
		Expect(out).ToNot(ContainSubstring("network:"))
	})

	It("uses the initramfs stage when there are no ssh keys", func() {
		m := &Model{disk: "/dev/sda", username: "kairos", password: "x"}
		out, err := RenderCloudConfig(m)
		Expect(err).ToNot(HaveOccurred())
		Expect(out).To(ContainSubstring("initramfs:"))
	})
})
