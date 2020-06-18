package gg

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"math/rand"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/oliveagle/jsonpath"
)

var charSet = []rune("1234567890abcdefghijklmnopqrstuvwxyz")

type Option func(gg *GroundGopher)

// WithHost sets the hostname of the API you want to test
func WithHost(host string) Option {
	return func(gg *GroundGopher) {
		gg.host = host
	}
}

// WithWorkers allows you to set the maximum number of parallel requests
func WithWorkers(i int) Option {
	return func(gg *GroundGopher) {
		gg.gophers = i
	}
}

// WithVerbose toggles the optional verbose output
func WithVerbose(b bool) Option {
	return func(gg *GroundGopher) {
		gg.verbose = b
	}
}

type GroundGopher struct {
	apiURL    *url.URL
	host      string
	variables []Variable
	gophers   int
	verbose   bool
	cache     map[string]string
	wg        sync.WaitGroup
}

// New returns a new gopher to run your tests
func New(opts ...Option) (*GroundGopher, error) {
	gg := GroundGopher{
		cache:   make(map[string]string),
		gophers: 10,
	}

	for _, opt := range opts {
		opt(&gg)
	}
	u, err := url.Parse(gg.host)
	if err != nil {
		return nil, err
	}
	gg.apiURL = u

	return &gg, nil
}

type Ctx struct {
	cache map[string]string
}

func (gg *GroundGopher) Set(k, v string) {
	gg.cache[k] = v
}

func (gg *GroundGopher) Get(k string) string {
	return gg.cache[k]
}

func (gg *Ctx) Set(k, v string) {
	gg.cache[k] = v
}

func (gg *Ctx) Get(k string) string {
	return gg.cache[k]
}

func (gg *GroundGopher) requestWorker(runCh chan Run, caseCh chan []Case, verbose bool) {
	defer gg.wg.Done()

	for cases := range caseCh {

		//prep the run
		run := Run{
			Failed:   false,
			WantFail: false,
		}

		c := Ctx{
			cache: make(map[string]string),
		}

		//prep the req
		var in In
		in.Client = http.Client{}
		in.Req = http.Request{
			Header: http.Header{},
		}
		u := *gg.apiURL
		in.Req.URL = &u
		in.Req.Body = nil

		disabled := false
		for _, cas := range cases {
			if cas.Disabled {
				disabled = true
				break
			}
			cas.Setup(&c, &in)
			run.Cases = append(run.Cases, cas.Name)
			if cas.WantFail {
				run.WantFail = true
			}
		}
		if disabled {
			continue
		}

		fmt.Printf("Starting run: %s\n", strings.Join(run.Cases, " "))
		ua := fmt.Sprintf("groundgopher-%s", randID())
		in.Req.Header.Set("User-Agent", ua)

		before := time.Now()
		resp, err := in.Client.Do(&in.Req)
		timeTaken := time.Since(before)
		if err != nil {
			//TODO: make error channel
			continue
		}

		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			//TODO: make error channel
			continue
		}
		resp.Body.Close()

		var out Out
		out.Body = body
		out.StatusCode = resp.StatusCode
		out.Resp = resp
		out.UserAgent = ua
		out.Duration = timeTaken
		run.Duration = timeTaken
		run.StatusCode = resp.StatusCode
		run.Body = string(body)

		for _, cas := range cases {
			res := cas.Validator(&c, out)
			res.Name = cas.Name
			res.WantedFail = cas.WantFail
			run.Results = append(run.Results, res)
			if res.Failed && cas.WantFail {
				break
			}
			if res.Failed && !cas.WantFail {
				run.Failed = true
			}
		}

		runCh <- run
	}
}

func (gg *GroundGopher) Run(t *testing.T) Report {
	rand.Seed(time.Now().Unix())

	var report Report

	runCh := make(chan Run)
	caseCh := Iter(gg.variables...)

	for i := 0; i < gg.gophers; i++ {
		gg.wg.Add(1)
		go gg.requestWorker(runCh, caseCh, gg.verbose)
	}
	go func() {
		gg.wg.Wait()
		if gg.verbose {
			fmt.Println("closing runCh")
		}
		close(runCh)
	}()

	for run := range runCh {

		if run.Failed && !run.WantFail {
			report.Failed = true
			report.Fails += 1
		} else {
			report.Successes += 1
		}
		report.Runs = append(report.Runs, run)
		report.Amount += 1
		report.TotalTime += run.Duration
	}

	return report
}

func (gg *GroundGopher) Add(v Variable) error {
	//TODO: some var verification
	gg.variables = append(gg.variables, v)
	return nil
}

type Case struct {
	Name      string
	Disabled  bool
	Setup     func(*Ctx, *In)
	Validator func(*Ctx, Out) Result
	WantFail  bool
}

type Variable struct {
	Name  string
	Cases []Case
}

type In struct {
	Req    http.Request
	Client http.Client
}

type Out struct {
	Resp       *http.Response
	Body       []byte
	StatusCode int
	Duration   time.Duration
	UserAgent  string
}

func (out *Out) JSONPathLookup(s string) (interface{}, error) {
	var data interface{}
	err := json.Unmarshal(out.Body, &data)
	if err != nil {
		return nil, fmt.Errorf("Unable to unmarshal json: %w", err)
	}
	return jsonpath.JsonPathLookup(data, s)
}

func (out *Out) JSONPathLookupString(s string) (string, error) {
	var data interface{}
	err := json.Unmarshal(out.Body, &data)
	if err != nil {
		return "", fmt.Errorf("Unable to unmarshal: %w", err)
	}
	res, err := jsonpath.JsonPathLookup(data, s)
	if err != nil {
		return "", fmt.Errorf("Unable to do lookup: %w", err)
	}
	resString, ok := res.(string)
	if !ok {
		return "", errors.New("Unable to cast value to string")
	}
	return resString, nil

}

type Result struct {
	Name       string
	Failed     bool
	WantedFail bool
	Reason     string
}

type Run struct {
	Cases      []string
	Results    []Result
	Body       string
	StatusCode int
	Duration   time.Duration
	WantFail   bool
	Failed     bool
}

type Report struct {
	Amount    int
	Runs      []Run
	Successes int
	Fails     int
	Failed    bool
	TotalTime time.Duration
}

func (r *Report) Summary() string {
	if !r.Failed {
		avgRespTime := int64(r.TotalTime) / int64(r.Amount)
		avgString := time.Duration(avgRespTime).String()
		return fmt.Sprintf("Completed %v requests in %s, average time: %s", r.Amount, r.TotalTime.String(), avgString)
	}
	return fmt.Sprintf("Completed %v (%v failed) requests in %s", r.Amount, r.Fails, r.TotalTime.String())
}

func randID() string {
	var output strings.Builder
	length := 10
	for i := 0; i < length; i++ {
		random := rand.Intn(len(charSet))
		randomChar := charSet[random]
		output.WriteRune(randomChar)
	}
	return output.String()
}
