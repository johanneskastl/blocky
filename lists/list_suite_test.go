package lists

import (
	"testing"

	. "github.com/0xERR0R/blocky/log"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestLists(t *testing.T) {
	ConfigureLogger(LevelFatal, FormatTypeText, true)
	RegisterFailHandler(Fail)
	RunSpecs(t, "Lists Suite")
}
