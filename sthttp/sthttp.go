package sthttp

import (
	"bytes"
	"encoding/xml"
	"errors"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/zpeters/speedtest/coords"
	"github.com/zpeters/speedtest/misc"
	"github.com/zpeters/speedtest/stxml"
)

// HTTPConfigTimeout is how long we'll wait for a config download to timeout
// var HTTPConfigTimeout = time.Duration(viper.GetDuration("httpconfigtimeout") * time.Second)

// // HTTPLatencyTimeout is how long we'll wait for a ping to timeout
// var HTTPLatencyTimeout = time.Duration(viper.GetDuration("httplatencytimeout") * time.Second)

// // HTTPDownloadTimeout is how long we'll wait for a download to timeout
// var HTTPDownloadTimeout = time.Duration(viper.GetDuration("httpdownloadtimeout") * time.Minute)

// CONFIG is our global config space
// var CONFIG Config

// Config struct holds our config (users current ip, lat, lon and isp)
type Config struct {
	IP  string
	Lat float64
	Lon float64
	Isp string
}

// Client define a Speedtest HTTP client
type Client struct {
	// Config defines the client configuration
	Config     *Config
	ConfigURL  string
	ServersURL string
	// HTTPConfigTimeout is how long we'll wait for a config download to timeout
	HTTPConfigTimeout time.Duration
	// HTTPLatencyTimeout is how long we'll wait for a ping to timeout
	HTTPLatencyTimeout time.Duration
	// HTTPDownloadTimeout is how long we'll wait for a download to timeout
	HTTPDownloadTimeout time.Duration
	Debug               bool
	AlgoType            string
	NumClosest          int
	NumLatencyTests     int
	Interface           string
	ReportChar          string
	Blacklist           string
}

// NewClient define a new Speedtest client.
func NewClient(configURL string, serversURL string, configTimeout time.Duration, latencyTimeout time.Duration, downloadTimeout time.Duration, debug bool, algotype string, numClosest int, numLatencyTests int, eth string, reportChar string, blacklist string) *Client {
	return &Client{
		ConfigURL:           configURL,
		ServersURL:          serversURL,
		HTTPConfigTimeout:   time.Duration(configTimeout * time.Second),
		HTTPLatencyTimeout:  time.Duration(latencyTimeout * time.Second),
		HTTPDownloadTimeout: time.Duration(downloadTimeout * time.Second),
		Debug:               debug,
		AlgoType:            algotype,
		NumClosest:          numClosest,
		NumLatencyTests:     numLatencyTests,
		Interface:           eth,
		ReportChar:          reportChar,
		Config:              &Config{},
	}

}

// Server struct is a speedtest candidate server
type Server struct {
	URL      string
	Lat      float64
	Lon      float64
	Name     string
	Country  string
	CC       string
	Sponsor  string
	ID       string
	Distance float64
	Latency  float64
}

// ByDistance allows us to sort servers by distance
type ByDistance []Server

func (server ByDistance) Len() int {
	return len(server)
}

func (server ByDistance) Less(i, j int) bool {
	return server[i].Distance < server[j].Distance
}

func (server ByDistance) Swap(i, j int) {
	server[i], server[j] = server[j], server[i]
}

// ByLatency allows us to sort servers by latency
type ByLatency []Server

func (server ByLatency) Len() int {
	return len(server)
}

func (server ByLatency) Less(i, j int) bool {
	return server[i].Latency < server[j].Latency
}

func (server ByLatency) Swap(i, j int) {
	server[i], server[j] = server[j], server[i]
}

// checkBlacklisted tests if the server is on the specified blacklist
func checkBlacklisted(blacklist string, server string) bool {
	var isBlacklisted bool = false
	bl := strings.Split(blacklist, ",")
	for b := range bl {
		blServer := strings.TrimSpace(bl[b])
		if server == blServer {
			isBlacklisted = true
		}
	}
	return isBlacklisted
}

// checkHTTP tests if http response is successful (200) or not
func checkHTTP(resp *http.Response) bool {
	var ok bool
	if resp.StatusCode != 200 {
		ok = false
	} else {
		ok = true
	}
	return ok
}

// GetConfig downloads the master config from speedtest.net
func (stClient *Client) GetConfig() (c Config, err error) {
	c = Config{}

	client := &http.Client{
		Timeout: stClient.HTTPConfigTimeout,
	}

	req, err := http.NewRequest("GET", stClient.ConfigURL, nil)
	if err != nil {
		return c, err
	}
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("User-Agent", "Unofficial CLI")

	resp, err := client.Do(req)
	if err != nil {
		return c, err
	}
	defer resp.Body.Close()
	if checkHTTP(resp) != true {
		log.Fatalf("Couldn't retrieve our config from speedtest.net: '%s'\n", resp.Status)
	}

	body, err := ioutil.ReadAll(resp.Body)

	cx := new(stxml.XMLConfigSettings)

	err = xml.Unmarshal(body, &cx)

	c.IP = cx.Client.IP
	c.Lat = misc.ToFloat(cx.Client.Lat)
	c.Lon = misc.ToFloat(cx.Client.Lon)
	c.Isp = cx.Client.Isp

	return c, err
}

// GetServers will get the full server list
func (stClient *Client) GetServers() (servers []Server, err error) {
	client := &http.Client{
		Timeout: stClient.HTTPConfigTimeout,
	}
	req, _ := http.NewRequest("GET", stClient.ServersURL, nil)
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("User-Agent", "Unofficial CLI")

	resp, err := client.Do(req)

	if err != nil {
		return servers, err
	}
	defer resp.Body.Close()

	body, err2 := ioutil.ReadAll(resp.Body)
	if err2 != nil {
		return servers, err2
	}

	s := new(stxml.ServerSettings)

	err3 := xml.Unmarshal(body, &s)
	if err3 != nil {
		return servers, err3
	}

	for xmlServer := range s.ServersContainer.XMLServers {
		// check if server is blacklisted
		if checkBlacklisted(stClient.Blacklist, s.ServersContainer.XMLServers[xmlServer].ID) == false {
			server := new(Server)
			server.URL = s.ServersContainer.XMLServers[xmlServer].URL
			server.Lat = misc.ToFloat(s.ServersContainer.XMLServers[xmlServer].Lat)
			server.Lon = misc.ToFloat(s.ServersContainer.XMLServers[xmlServer].Lon)
			server.Name = s.ServersContainer.XMLServers[xmlServer].Name
			server.Country = s.ServersContainer.XMLServers[xmlServer].Country
			server.CC = s.ServersContainer.XMLServers[xmlServer].CC
			server.Sponsor = s.ServersContainer.XMLServers[xmlServer].Sponsor
			server.ID = s.ServersContainer.XMLServers[xmlServer].ID
			servers = append(servers, *server)
		}
	}
	return servers, nil
}

// GetClosestServers takes the full server list and sorts by distance
func (stClient *Client) GetClosestServers(servers []Server) []Server {
	if stClient.Debug {
		log.Printf("Sorting all servers by distance...\n")
	}

	myCoords := coords.Coordinate{
		Lat: stClient.Config.Lat,
		Lon: stClient.Config.Lon,
	}
	for server := range servers {
		theirlat := servers[server].Lat
		theirlon := servers[server].Lon
		theirCoords := coords.Coordinate{Lat: theirlat, Lon: theirlon}

		servers[server].Distance = coords.HsDist(coords.DegPos(myCoords.Lat, myCoords.Lon), coords.DegPos(theirCoords.Lat, theirCoords.Lon))
	}

	sort.Sort(ByDistance(servers))

	return servers
}

// GetLatencyURL will return the proper url for the latency
// test file when given a server name
func (stClient *Client) GetLatencyURL(server Server) string {
	u := server.URL
	splits := strings.Split(u, "/")
	baseURL := strings.Join(splits[1:len(splits)-1], "/")
	latencyURL := "http:/" + baseURL + "/latency.txt"
	return latencyURL
}

// GetLatency will test the latency (ping) the given server NUMLATENCYTESTS times and return either the lowest or average depending on what algorithm is set
func (stClient *Client) GetLatency(server Server, url string) (result float64, err error) {
	var latency time.Duration
	var minLatency time.Duration
	var avgLatency time.Duration

	// url := stClient.GetLatencyURL(server)

	for i := 0; i < stClient.NumLatencyTests; i++ {
		var failed bool
		var finish time.Time

		if stClient.Debug {
			log.Printf("Testing latency: %s (%s)\n", server.Name, server.Sponsor)
		}

		start := time.Now()

		client, err := stClient.getHTTPClient()
		if err != nil {
			return result, err
		}
		req, _ := http.NewRequest("GET", url, nil)
		req.Header.Set("Cache-Control", "no-cache")
		req.Header.Set("User-Agent", "Unofficial CLI")

		resp, err := client.Do(req)

		if err != nil {
			return result, err
		}

		defer resp.Body.Close()
		finish = time.Now()
		_, err2 := ioutil.ReadAll(resp.Body)
		if err2 != nil {
			return result, err
		}

		if failed == true {
			latency = 1 * time.Minute
		} else {
			latency = finish.Sub(start)
		}

		if stClient.Debug {
			log.Printf("\tRun took: %v\n", latency)
		}

		if stClient.AlgoType == "max" {
			if minLatency == 0 {
				minLatency = latency
			} else if latency < minLatency {
				minLatency = latency
			}
		} else {
			avgLatency = avgLatency + latency
		}

	}

	if stClient.AlgoType == "max" {
		result = float64(time.Duration(minLatency.Nanoseconds())*time.Nanosecond) / 1000000
	} else {
		result = float64(time.Duration(avgLatency.Nanoseconds())*time.Nanosecond) / 1000000 / float64(stClient.NumLatencyTests)
	}

	return result, nil

}

// GetFastestServer test all servers until we find numServers that
// respond, then find the fastest of them.  Some servers show up in the
// master list but timeout or are "corrupt" therefore we bump their
// latency to something really high (1 minute) and they will drop out of
// this test
func (stClient *Client) GetFastestServer(servers []Server) Server {
	var successfulServers []Server

	for server := range servers {
		if stClient.Debug {
			log.Printf("Doing %d runs of %v\n", stClient.NumClosest, servers[server])
		}
		latency, err := stClient.GetLatency(servers[server], stClient.GetLatencyURL(servers[server]))
		if err != nil {
			log.Fatal(err)
		}

		if stClient.Debug {
			log.Printf("Total runs took: %v\n", latency)
		}

		if latency > float64(time.Duration(1*time.Minute)) {
			if stClient.Debug {
				log.Printf("Server %d was too slow, skipping...\n", server)
			}
		} else {
			if stClient.Debug {
				log.Printf("Server latency was ok %f adding to successful servers list", latency)
			}
			successfulServers = append(successfulServers, servers[server])
			successfulServers[server].Latency = latency
		}

		if len(successfulServers) == stClient.NumClosest {
			break
		}
	}

	sort.Sort(ByLatency(successfulServers))
	if stClient.Debug {
		log.Printf("Server: %v is the fastest server\n", successfulServers[0])
	}
	return successfulServers[0]
}

// DownloadSpeed measures the mbps of downloading a URL
func (stClient *Client) DownloadSpeed(url string) (speed float64, err error) {
	start := time.Now()
	if stClient.Debug {
		log.Printf("Starting test at: %s\n", start)
	}
	client, err := stClient.getHTTPClient()
	if err != nil {
		return 0, err
	}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("User-Agent", "Unofficial CLI")

	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Fatal(err)
	}
	bodyLen := len(body)
	finish := time.Now()

	bits := float64(bodyLen * 8)
	megabits := bits / float64(1000) / float64(1000)
	seconds := finish.Sub(start).Seconds()
	mbps := megabits / float64(seconds)

	return mbps, err
}

// UploadSpeed measures the mbps to http.Post to a URL
func (stClient *Client) UploadSpeed(url string, mimetype string, data []byte) (speed float64, err error) {
	buf := bytes.NewBuffer(data)

	start := time.Now()
	if stClient.Debug {
		log.Printf("Starting test at: %s\n", start)
		log.Printf("Starting test at: %d (nano)\n", start.UnixNano())
	}

	client, err := stClient.getHTTPClient()
	if err != nil {
		return 0, err
	}
	resp, err := client.Post(url, mimetype, buf)
	finish := time.Now()
	if err != nil {
		return 0, err
	}

	defer resp.Body.Close()
	_, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}

	if stClient.Debug {
		log.Printf("Finishing test at: %s\n", finish)
		log.Printf("Finishing test at: %d (nano)\n", finish.UnixNano())
		log.Printf("Took: %d (nano)\n", finish.Sub(start).Nanoseconds())
	}

	bits := float64(len(data) * 8)
	megabits := bits / float64(1000) / float64(1000)
	seconds := finish.Sub(start).Seconds()

	mbps := megabits / float64(seconds)
	return mbps, nil
}

func (stClient *Client) getSourceIP() (string, error) {
	interfaceOption := stClient.Interface
	if interfaceOption == "" {
		return "", nil
	}

	// does it look like an IP address?
	if net.ParseIP(interfaceOption) != nil {
		return interfaceOption, nil
	}

	// assume that it is the name of an interface
	iface, err := net.InterfaceByName(interfaceOption)
	if err != nil {
		return "", err
	}

	addrs, err := iface.Addrs()
	if err != nil {
		return "", err
	}

	for _, addr := range addrs {
		switch v := addr.(type) {
		case *net.IPNet:
			// fixme: IPv6 support is missing
			if v.IP.To4() != nil {
				return v.IP.String(), nil
			}
		case *net.IPAddr:
			if v.IP.To4() != nil {
				return v.IP.String(), nil
			}
		}
	}

	return "", errors.New("no address found")
}

func (stClient *Client) getHTTPClient() (*http.Client, error) {
	var dialer net.Dialer

	sourceIP, err := stClient.getSourceIP()
	if err != nil {
		return nil, err
	}
	if sourceIP != "" {
		bindAddrIP, err := net.ResolveIPAddr("ip", sourceIP)
		if err != nil {
			return nil, err
		}
		bindAddr := net.TCPAddr{
			IP: bindAddrIP.IP,
		}
		dialer = net.Dialer{
			LocalAddr: &bindAddr,
			Timeout:   stClient.HTTPConfigTimeout,
			KeepAlive: stClient.HTTPConfigTimeout,
		}
	} else {
		dialer = net.Dialer{
			Timeout:   stClient.HTTPConfigTimeout,
			KeepAlive: stClient.HTTPConfigTimeout,
		}
	}
	transport := &http.Transport{
		Proxy:               http.ProxyFromEnvironment,
		Dial:                dialer.Dial,
		TLSHandshakeTimeout: stClient.HTTPConfigTimeout,
	}
	client := &http.Client{
		Timeout:   stClient.HTTPConfigTimeout,
		Transport: transport,
	}
	return client, nil
}
