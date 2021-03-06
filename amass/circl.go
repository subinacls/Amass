// Copyright 2017 Jeff Foley. All rights reserved.
// Use of this source code is governed by Apache 2 LICENSE that can be found in the LICENSE file.

package amass

import (
	"bufio"
	"encoding/json"
	"strings"
	"time"

	"github.com/OWASP/Amass/amass/utils"
)

// CIRCL is the Service that handles access to the CIRCL data source.
type CIRCL struct {
	BaseService

	API        *APIKey
	SourceType string
	RateLimit  time.Duration
}

// NewCIRCL returns he object initialized, but not yet started.
func NewCIRCL(e *Enumeration) *CIRCL {
	c := &CIRCL{
		SourceType: API,
		RateLimit:  time.Second,
	}

	c.BaseService = *NewBaseService(e, "CIRCL", c)
	return c
}

// OnStart implements the Service interface
func (c *CIRCL) OnStart() error {
	c.BaseService.OnStart()

	c.API = c.Enum().Config.GetAPIKey(c.String())
	if c.API == nil || c.API.Username == "" || c.API.Password == "" {
		c.Enum().Log.Printf("%s: API key data was not provided", c.String())
	}
	go c.startRootDomains()
	go c.processRequests()
	return nil
}

func (c *CIRCL) processRequests() {
	for {
		select {
		case <-c.PauseChan():
			<-c.ResumeChan()
		case <-c.Quit():
			return
		case <-c.RequestChan():
			// This data source just throws away the checked DNS names
			c.SetActive()
		}
	}
}

func (c *CIRCL) startRootDomains() {
	// Look at each domain provided by the config
	for _, domain := range c.Enum().Config.Domains() {
		c.executeQuery(domain)
		// Honor the rate limit
		time.Sleep(c.RateLimit)
	}
}

func (c *CIRCL) executeQuery(domain string) {
	if c.API == nil || c.API.Username == "" || c.API.Password == "" {
		return
	}

	c.SetActive()
	url := c.restURL(domain)
	headers := map[string]string{"Content-Type": "application/json"}
	page, err := utils.RequestWebPage(url, nil, headers, c.API.Username, c.API.Password)
	if err != nil {
		c.Enum().Log.Printf("%s: %s: %v", c.String(), url, err)
		return
	}

	c.passiveDNSJSON(page, domain)
}

func (c *CIRCL) restURL(domain string) string {
	return "https://www.circl.lu/pdns/query/" + domain
}

func (c *CIRCL) passiveDNSJSON(page, domain string) {
	var unique []string

	c.SetActive()
	re := c.Enum().Config.DomainRegex(domain)
	scanner := bufio.NewScanner(strings.NewReader(page))
	for scanner.Scan() {
		// Get the next line of JSON
		line := scanner.Text()
		if line == "" {
			continue
		}

		var j struct {
			Name string `json:"rrname"`
		}
		err := json.Unmarshal([]byte(line), &j)
		if err != nil {
			continue
		}
		if re.MatchString(j.Name) {
			unique = utils.UniqueAppend(unique, j.Name)
		}
	}

	for _, name := range unique {
		c.Enum().NewNameEvent(&Request{
			Name:   name,
			Domain: domain,
			Tag:    API,
			Source: c.String(),
		})
	}
}
