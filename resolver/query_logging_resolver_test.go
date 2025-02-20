package resolver

import (
	"bufio"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	"github.com/0xERR0R/blocky/querylog"

	"github.com/0xERR0R/blocky/config"
	. "github.com/0xERR0R/blocky/model"
	"github.com/0xERR0R/blocky/util"

	"github.com/miekg/dns"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"
)

type SlowMockWriter struct {
	entries []*querylog.LogEntry
}

func (m *SlowMockWriter) Write(entry *querylog.LogEntry) {
	m.entries = append(m.entries, entry)

	time.Sleep(time.Millisecond)
}

func (m *SlowMockWriter) CleanUp() {
}

var _ = Describe("QueryLoggingResolver", func() {
	var (
		sut        *QueryLoggingResolver
		sutConfig  config.QueryLogConfig
		err        error
		resp       *Response
		m          *resolverMock
		tmpDir     string
		mockAnswer *dns.Msg
	)

	BeforeEach(func() {
		mockAnswer = new(dns.Msg)
		tmpDir, err = ioutil.TempDir("", "queryLoggingResolver")
		Expect(err).Should(Succeed())
	})

	JustBeforeEach(func() {
		sut = NewQueryLoggingResolver(sutConfig).(*QueryLoggingResolver)
		m = &resolverMock{}
		m.On("Resolve", mock.Anything).Return(&Response{Res: mockAnswer, Reason: "reason"}, nil)
		sut.Next(m)
	})
	AfterEach(func() {
		Expect(err).Should(Succeed())
		_ = os.RemoveAll(tmpDir)
	})

	Describe("Process request", func() {

		When("Resolver has no configuration", func() {
			BeforeEach(func() {
				sutConfig = config.QueryLogConfig{
					CreationAttempts: 1,
					CreationCooldown: config.Duration(time.Millisecond),
				}
			})
			It("should process request without query logging", func() {
				resp, err = sut.Resolve(newRequest("example.com.", dns.TypeA))

				m.AssertExpectations(GinkgoT())
				Expect(resp.RType).Should(Equal(ResponseTypeRESOLVED))
			})
		})
		When("Configuration with logging per client", func() {
			BeforeEach(func() {
				sutConfig = config.QueryLogConfig{
					Target:           tmpDir,
					Type:             config.QueryLogTypeCsvClient,
					CreationAttempts: 1,
					CreationCooldown: config.Duration(time.Millisecond),
				}
				mockAnswer, _ = util.NewMsgWithAnswer("example.com.", 300, dns.TypeA, "123.122.121.120")
			})
			It("should create a log file per client", func() {
				By("request from client 1", func() {
					resp, err = sut.Resolve(newRequestWithClient("example.com.", dns.TypeA, "192.168.178.25", "client1"))
					Expect(err).Should(Succeed())
				})
				By("request from client 2, has name with special chars, should be escaped", func() {
					resp, err = sut.Resolve(newRequestWithClient("example.com.", dns.TypeA, "192.168.178.26", "cl/ient2\\$%&test"))
					Expect(err).Should(Succeed())
				})

				m.AssertExpectations(GinkgoT())

				By("check log for client1", func() {
					Eventually(func(g Gomega) {
						csvLines, err := readCsv(filepath.Join(tmpDir, fmt.Sprintf("%s_client1.log", time.Now().Format("2006-01-02"))))

						g.Expect(err).Should(Succeed())
						g.Expect(csvLines).Should(Not(BeEmpty()))
						g.Expect(csvLines[0][1]).Should(Equal("192.168.178.25"))
						g.Expect(csvLines[0][2]).Should(Equal("client1"))
						g.Expect(csvLines[0][4]).Should(Equal("reason"))
						g.Expect(csvLines[0][5]).Should(Equal("A (example.com.)"))
						g.Expect(csvLines[0][6]).Should(Equal("A (123.122.121.120)"))
					}, "1s").Should(Succeed())

				})

				By("check log for client2", func() {
					Eventually(func(g Gomega) {
						csvLines, err := readCsv(filepath.Join(tmpDir,
							fmt.Sprintf("%s_cl_ient2_test.log", time.Now().Format("2006-01-02"))))

						g.Expect(err).Should(Succeed())
						g.Expect(csvLines).Should(HaveLen(1))
						g.Expect(csvLines[0][1]).Should(Equal("192.168.178.26"))
						g.Expect(csvLines[0][2]).Should(Equal("cl/ient2\\$%&test"))
						g.Expect(csvLines[0][4]).Should(Equal("reason"))
						g.Expect(csvLines[0][5]).Should(Equal("A (example.com.)"))
						g.Expect(csvLines[0][6]).Should(Equal("A (123.122.121.120)"))
					}, "1s").Should(Succeed())
				})
			})
		})
		When("Configuration with logging in one file for all clients", func() {
			BeforeEach(func() {
				sutConfig = config.QueryLogConfig{
					Target:           tmpDir,
					Type:             config.QueryLogTypeCsv,
					CreationAttempts: 1,
					CreationCooldown: config.Duration(time.Millisecond),
				}
				mockAnswer, _ = util.NewMsgWithAnswer("example.com.", 300, dns.TypeA, "123.122.121.120")
			})
			It("should create one log file for all clients", func() {
				By("request from client 1", func() {
					resp, err = sut.Resolve(newRequestWithClient("example.com.", dns.TypeA, "192.168.178.25", "client1"))
					Expect(err).Should(Succeed())
				})
				By("request from client 2, has name with special chars, should be escaped", func() {
					resp, err = sut.Resolve(newRequestWithClient("example.com.", dns.TypeA, "192.168.178.26", "client2"))
					Expect(err).Should(Succeed())
				})

				m.AssertExpectations(GinkgoT())

				By("check log", func() {
					Eventually(func(g Gomega) {
						csvLines, err := readCsv(filepath.Join(tmpDir, fmt.Sprintf("%s_ALL.log", time.Now().Format("2006-01-02"))))

						g.Expect(err).Should(Succeed())
						g.Expect(csvLines).Should(HaveLen(2))
						// client1 -> first line
						g.Expect(csvLines[0][1]).Should(Equal("192.168.178.25"))
						g.Expect(csvLines[0][2]).Should(Equal("client1"))
						g.Expect(csvLines[0][4]).Should(Equal("reason"))
						g.Expect(csvLines[0][5]).Should(Equal("A (example.com.)"))
						g.Expect(csvLines[0][6]).Should(Equal("A (123.122.121.120)"))

						// client2 -> second line
						g.Expect(csvLines[1][1]).Should(Equal("192.168.178.26"))
						g.Expect(csvLines[1][2]).Should(Equal("client2"))
						g.Expect(csvLines[1][4]).Should(Equal("reason"))
						g.Expect(csvLines[1][5]).Should(Equal("A (example.com.)"))
						g.Expect(csvLines[1][6]).Should(Equal("A (123.122.121.120)"))
					}, "1s").Should(Succeed())

				})
			})
		})
	})

	Describe("Slow writer", func() {
		When("writer is too slow", func() {
			BeforeEach(func() {
				sutConfig = config.QueryLogConfig{
					Type:             config.QueryLogTypeNone,
					CreationAttempts: 1,
					CreationCooldown: config.Duration(time.Millisecond),
				}
			})
			It("should drop messages", func() {
				mockWriter := &SlowMockWriter{}
				sut.writer = mockWriter

				// run 10000 requests
				for i := 0; i < 10000; i++ {
					resp, err = sut.Resolve(newRequestWithClient("example.com.", dns.TypeA, "192.168.178.25", "client1"))
					Expect(err).Should(Succeed())
				}

				// log channel is full
				Expect(sut.logChan).Should(Not(BeEmpty()))
			})

		})
	})

	Describe("Configuration output", func() {
		When("resolver is enabled", func() {
			BeforeEach(func() {
				sutConfig = config.QueryLogConfig{
					Target:           tmpDir,
					Type:             config.QueryLogTypeCsvClient,
					LogRetentionDays: 0,
					CreationAttempts: 1,
					CreationCooldown: config.Duration(time.Millisecond),
				}
			})
			It("should return configuration", func() {
				c := sut.Configuration()
				Expect(len(c) > 1).Should(BeTrue())
			})
		})
	})

	Describe("Clean up of query log directory", func() {
		When("fallback logger is enabled, log retention is enabled", func() {
			It("should do nothing", func() {

				sut := NewQueryLoggingResolver(config.QueryLogConfig{
					LogRetentionDays: 7,
					Type:             config.QueryLogTypeConsole,
					CreationAttempts: 1,
					CreationCooldown: config.Duration(time.Millisecond),
				}).(*QueryLoggingResolver)

				sut.doCleanUp()
			})
		})
		When("log directory contains old files", func() {
			It("should remove files older than defined log retention", func() {

				// create 2 files, 7 and 8 days old
				dateBefore7Days := time.Now().AddDate(0, 0, -7)
				dateBefore8Days := time.Now().AddDate(0, 0, -8)

				f1, err := os.Create(filepath.Join(tmpDir, fmt.Sprintf("%s-test.log", dateBefore7Days.Format("2006-01-02"))))
				Expect(err).Should(Succeed())

				f2, err := os.Create(filepath.Join(tmpDir, fmt.Sprintf("%s-test.log", dateBefore8Days.Format("2006-01-02"))))
				Expect(err).Should(Succeed())

				sut := NewQueryLoggingResolver(config.QueryLogConfig{
					Target:           tmpDir,
					Type:             config.QueryLogTypeCsv,
					LogRetentionDays: 7,
					CreationAttempts: 1,
					CreationCooldown: config.Duration(time.Millisecond),
				})

				sut.(*QueryLoggingResolver).doCleanUp()

				// file 1 exist
				_, err = os.Stat(f1.Name())
				Expect(err).Should(Succeed())

				// file 2 was deleted
				_, err = os.Stat(f2.Name())
				Expect(err).Should(HaveOccurred())
				Expect(os.IsNotExist(err)).Should(BeTrue())
			})
		})
	})

})

var _ = Describe("Wrong target configuration", func() {
	When("mysql database path is wrong", func() {
		It("should use fallback", func() {
			sutConfig := config.QueryLogConfig{
				Target:           "dummy",
				Type:             config.QueryLogTypeMysql,
				CreationAttempts: 1,
				CreationCooldown: config.Duration(time.Millisecond),
			}
			resolver := NewQueryLoggingResolver(sutConfig)
			loggingResolver := resolver.(*QueryLoggingResolver)
			Expect(loggingResolver.logType).Should(Equal(config.QueryLogTypeConsole))
		})
	})

	When("postgresql database path is wrong", func() {
		It("should use fallback", func() {
			sutConfig := config.QueryLogConfig{
				Target:           "dummy",
				Type:             config.QueryLogTypePostgresql,
				CreationAttempts: 1,
				CreationCooldown: config.Duration(time.Millisecond),
			}
			resolver := NewQueryLoggingResolver(sutConfig)
			loggingResolver := resolver.(*QueryLoggingResolver)
			Expect(loggingResolver.logType).Should(Equal(config.QueryLogTypeConsole))
		})
	})
})

func readCsv(file string) ([][]string, error) {
	var result [][]string

	csvFile, err := os.Open(file)
	if err != nil {
		return nil, err
	}

	reader := csv.NewReader(bufio.NewReader(csvFile))
	reader.Comma = '\t'

	for {
		line, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			return nil, err
		}

		result = append(result, line)
	}

	return result, nil
}
