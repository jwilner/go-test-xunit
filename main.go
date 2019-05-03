package main

import (
	"bufio"
	"context"
	"encoding/json"
	"encoding/xml"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"strings"
)

type testEvent struct {
	Test    string
	Elapsed float64

	Action  string
	Package string
	Output  string
}

func runTests() (events []testEvent, exit int, err error) {
	args := []string{"test", "-json"}
	for _, arg := range os.Args[1:] {
		if arg != "-json" {
			args = append(args, arg)
		}
	}

	var f *os.File
	if f, err = ioutil.TempFile("", "go-xunit"); err != nil {
		return
	}
	defer func() {
		_ = f.Close()
		_ = os.Remove(f.Name())
	}()

	cmd := exec.CommandContext(context.Background(), "go", args...)
	cmd.Stdout = f

	if err = cmd.Run(); err != nil {
		e, ok := err.(*exec.ExitError)
		if !ok {
			return
		}
		exit = e.ExitCode()
	}

	if _, err = f.Seek(0, io.SeekStart); err != nil {
		return
	}

	scnr := bufio.NewScanner(f)
	for scnr.Scan() {
		var ev testEvent
		if err = json.Unmarshal(scnr.Bytes(), &ev); err != nil {
			return
		}
		events = append(events, ev)
	}

	err = scnr.Err()
	return
}

func report(events []testEvent, w io.Writer) error {
	var (
		pkgOrder []string
		pkgs     = make(map[string][]testEvent)
	)
	for _, ev := range events {
		if _, ok := pkgs[ev.Package]; !ok {
			pkgOrder = append(pkgOrder, ev.Package)
		}
		pkgs[ev.Package] = append(pkgs[ev.Package], ev)
	}

	type testFailure struct {
		Type    string `xml:"type,attr"`
		Message string `xml:"message,attr"`
		Output  string `xml:",cdata"`
	}

	type testCase struct {
		Classname string       `xml:"classname,attr"`
		Name      string       `xml:"name,attr"`
		Time      float64      `xml:"time,attr"`
		Failure   *testFailure `xml:"failure"`
	}

	type testSuite struct {
		Name      string      `xml:"name,attr"`
		Tests     int         `xml:"tests,attr"`
		Errors    int         `xml:"errors,attr"`
		Failures  int         `xml:"failures,attr"`
		Skip      int         `xml:"skip,attr"`
		TestCases []*testCase `xml:"testcase"`
	}

	var testSuites []*testSuite
	for _, pkg := range pkgOrder {
		suite := &testSuite{Name: pkg}
		testSuites = append(testSuites, suite)
		failures := make(map[string]*testFailure)

		for _, ev := range pkgs[pkg] {
			if ev.Test == "" {
				continue
			}
			switch ev.Action {
			case "pass":
				suite.Tests++
				suite.TestCases = append(suite.TestCases, &testCase{
					Classname: pkg,
					Name:      ev.Test,
					Time:      ev.Elapsed,
				})
			case "fail":
				suite.Tests++
				suite.Failures++
				f := &testFailure{Type: "go.error", Message: "error"}
				suite.TestCases = append(suite.TestCases, &testCase{
					Classname: pkg,
					Name:      ev.Test,
					Time:      ev.Elapsed,
					Failure:   f,
				})
				failures[ev.Test] = f
			case "skip":
				suite.Tests++
				suite.Skip++
				suite.TestCases = append(suite.TestCases, &testCase{
					Classname: pkg,
					Name:      ev.Test,
					Time:      ev.Elapsed,
				})
			}
		}

		if len(failures) == 0 {
			continue
		}

		for _, ev := range events {
			if ev.Action != "output" {
				continue
			}
			if _, ok := failures[ev.Test]; !ok {
				continue
			}
			if !strings.HasPrefix(ev.Output, "=== RUN") &&
				!strings.Contains(ev.Output, "--- FAIL") {
				failures[ev.Test].Output += ev.Output
			}
		}

	}
	if _, err := w.Write([]byte(xml.Header)); err != nil {
		return err
	}
	enc := xml.NewEncoder(w)
	enc.Indent("", "    ")
	return enc.Encode(struct {
		XMLName    xml.Name     `xml:"testsuites"`
		TestSuites []*testSuite `xml:"testsuite"`
	}{
		TestSuites: testSuites,
	})
}

func main() {
	events, exit, err := runTests()
	if err != nil {
		log.Fatal(err)
	}

	if err := report(events, os.Stdout); err != nil {
		log.Fatal(err)
	}

	os.Exit(exit)
}
