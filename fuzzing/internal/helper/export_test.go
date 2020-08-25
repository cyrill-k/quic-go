package helper

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("exporting", func() {
	var dir string

	BeforeEach(func() {
		var err error
		dir, err = ioutil.TempDir("", "fuzzing-helper")
		Expect(err).ToNot(HaveOccurred())
		fmt.Fprintf(GinkgoWriter, "Created temporary directory %s", dir)
	})

	AfterEach(func() {
		Expect(dir).ToNot(BeEmpty())
		Expect(os.RemoveAll(dir)).To(Succeed())
	})

	It("writes a file", func() {
		const data = "lorem ipsum"
		// calculated by running sha1sum on the generated file
		const expectedShaSum = "bfb7759a67daeb65410490b4d98bb9da7d1ea2ce"
		Expect(WriteCorpusFile(dir, []byte("lorem ipsum"))).To(Succeed())
		path := filepath.Join(dir, expectedShaSum)
		Expect(path).To(BeARegularFile())
		b, err := ioutil.ReadFile(path)
		Expect(err).ToNot(HaveOccurred())
		Expect(string(b)).To(Equal(data))
	})

	It("writes a file and prepends data", func() {
		const data = "lorem ipsum"
		// calculated by running sha1sum on the generated file
		const expectedShaSum = "523f5cab80fab0c7889dbf50dd310ab8c8879f9c"
		const prefixLen = 7
		Expect(WriteCorpusFileWithPrefix(dir, []byte("lorem ipsum"), prefixLen)).To(Succeed())
		path := filepath.Join(dir, expectedShaSum)
		Expect(path).To(BeARegularFile())
		b, err := ioutil.ReadFile(path)
		Expect(err).ToNot(HaveOccurred())
		Expect(b[:prefixLen]).To(Equal(make([]byte, prefixLen)))
		Expect(string(b[prefixLen:])).To(Equal(data))
	})

	It("creates the directory, if it doesn't yet", func() {
		subdir := filepath.Join(dir, "corpus")
		Expect(subdir).ToNot(BeADirectory())
		Expect(WriteCorpusFile(subdir, []byte("lorem ipsum"))).To(Succeed())
		Expect(subdir).To(BeADirectory())
	})
})
