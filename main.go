package main

import (
	"errors"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

// Conf file named update.json
type Conf struct {
	Providers []string `json:"providers"`
	Filter    []string `json:"filter"`
}

// Server type
type Server struct {
	Method     string `json:"method"`
	Password   string `json:"password"`
	Plugin     string `json:"plugin"`
	PluginArgs string `json:"plugin_args"`
	PluginOpts string `json:"plugin_opts"`
	Remarks    string `json:"remarks"`
	Server     string `json:"server"`
	ServerPort int    `json:"server_port"`
	Timeout    int    `json:"timeout"`
}

// SSGui GUI json
type SSGui struct {
	AutoCheckUpdate        bool     `json:"autoCheckUpdate"`
	AvailabilityStatistics bool     `json:"availabilityStatistics"`
	CheckPreRelease        bool     `json:"checkPreRelease"`
	Configs                []Server `json:"configs"`
	Enabled                bool     `json:"enabled"`
	Global                 bool     `json:"global"`
	Hotkey                 struct {
		RegHotkeysAtStartup   bool   `json:"RegHotkeysAtStartup"`
		ServerMoveDown        string `json:"ServerMoveDown"`
		ServerMoveUp          string `json:"ServerMoveUp"`
		ShowLogs              string `json:"ShowLogs"`
		SwitchAllowLan        string `json:"SwitchAllowLan"`
		SwitchSystemProxy     string `json:"SwitchSystemProxy"`
		SwitchSystemProxyMode string `json:"SwitchSystemProxyMode"`
	} `json:"hotkey"`
	Index            int  `json:"index"`
	IsDefault        bool `json:"isDefault"`
	IsVerboseLogging bool `json:"isVerboseLogging"`
	LocalPort        int  `json:"localPort"`
	LogViewer        struct {
		BackgroundColor string `json:"BackgroundColor"`
		Font            string `json:"Font"`
		TextColor       string `json:"TextColor"`
		ToolbarShown    bool   `json:"toolbarShown"`
		TopMost         bool   `json:"topMost"`
		WrapText        bool   `json:"wrapText"`
	} `json:"logViewer"`
	PacURL       string `json:"pacUrl"`
	PortableMode bool   `json:"portableMode"`
	Proxy        struct {
		ProxyPort    int    `json:"proxyPort"`
		ProxyServer  string `json:"proxyServer"`
		ProxyTimeout int    `json:"proxyTimeout"`
		ProxyType    int    `json:"proxyType"`
		UseProxy     bool   `json:"useProxy"`
	} `json:"proxy"`
	SecureLocalPac bool        `json:"secureLocalPac"`
	ShareOverLan   bool        `json:"shareOverLan"`
	Strategy       interface{} `json:"strategy"`
	UseOnlinePac   bool        `json:"useOnlinePac"`
}

// readConf read update.json
func readConf() (Conf, error) {
	cb, err := ioutil.ReadFile("update.json")
	if err != nil {
		return Conf{}, err
	}
	var conf Conf
	json.Unmarshal(cb, &conf)
	return conf, nil
}

// readSSGui read gui json
func readSSGui() (SSGui, error) {
	cb, err := ioutil.ReadFile("gui-config.json")
	if err != nil {
		return SSGui{}, err
	}
	var gui SSGui
	json.Unmarshal(cb, &gui)
	return gui, nil
}

// surgeFromConf match surge urls
func surgeFromConf(conf string) ([]string, error) {
	re, err := regexp.Compile("\\[Proxy\\]([\\s\\S]*?)\\[Proxy Group\\]")
	if err != nil {
		return []string{}, err
	}
	submatch := re.FindSubmatch([]byte(conf))
	if len(submatch) == 2 {
		return strings.Split(string(submatch[1]), "\n"), nil
	}
	return []string{}, errors.New("could not match [Proxy] in profile") 
}

// surge2SS convert surge style url to ss-gui format
func surge2SS(surge string) Server {
	regex, _ := regexp.Compile("(.*?)\\s*=\\s*custom,(.*?),(.*?),(.*?),(.*?),")
	obfsRegex, _ := regexp.Compile("obfs-host\\s*=\\s*(.*?)(?:,|$)")
	obfsTypeRegex, _ := regexp.Compile("obfs\\s*=\\s*(.*?)(?:,|$)")
	var res Server
	params := regex.FindSubmatch([]byte(surge))
	if len(params) == 6 {
		res.Server = strings.TrimSpace(string(params[2]))
		res.ServerPort, _ = strconv.Atoi(strings.TrimSpace(string(params[3])))
		res.Password = strings.TrimSpace(string(params[5]))
		res.Method = strings.TrimSpace(string(params[4]))
		res.Remarks = strings.TrimSpace(string(params[1]))
		res.Timeout = 5
		obfsType := obfsTypeRegex.FindSubmatch([]byte(surge))
		if len(obfsType) == 2 {
			res.Plugin = "obfs-local"
			res.PluginOpts = "obfs=" + strings.TrimSpace(string(obfsType[1]))
			obfsParams := obfsRegex.FindSubmatch([]byte(surge))
			if len(obfsParams) == 2 {
				res.PluginOpts += ";obfs-host=" + strings.TrimSpace(string(obfsParams[1]))
			}
		}
	}
	return res
}

// Result of convertion
type Result struct {
	Success []string
	Fromat  []string
	Network []string
}

func main() {
	conf, err := readConf()
	if err != nil {
		fmt.Printf("无法读取update.json，错误：%s", err.Error())
		return
	}
	gui, err := readSSGui()
	if err != nil {
		fmt.Printf("无法读取gui-config.json，错误： %s", err.Error())
		return
	}

	providers := conf.Providers
	fmt.Println(fmt.Sprintf("成功读取到%d个托管配置，开始下载...", len(providers)))
	filters := conf.Filter
	fmt.Println(fmt.Sprintf("关键字过滤：%s", strings.Join(filters, " | ")))
	var result Result
	var remotes []string
	client := &http.Client{}
	var wg sync.WaitGroup
	wg.Add(len(providers))
	for i := 0; i < len(providers); i++ {
		go func(url string) {
			defer wg.Done()
			request, err := http.NewRequest("GET", url, nil)
			request.Header.Add("User-Agent", "Surge/1166 CFNetwork/955.1.2 Darwin/18.0.0")
			if err != nil {
				fmt.Println(err)
				return
			}
			resp, err := client.Do(request)
			if err != nil {
				fmt.Println("获取托管失败", err)
				result.Network = append(result.Network, url)
				return
			}
			defer resp.Body.Close()
			body, err := ioutil.ReadAll(resp.Body)
			if err != nil {
				fmt.Println(err)
				return
			}
			remotes = append(remotes, string(body))
		}(providers[i])
	}
	wg.Wait()
	var servers []Server
	for k := 0; k < len(remotes); k++ {
		urls, err := surgeFromConf(remotes[k])
		if err != nil {
			fmt.Printf("无法获取[Proxy]内容，错误：%s", err.Error())
			continue
		}
		if urls == nil {
			result.Fromat = append(result.Fromat, providers[k])
			continue
		}
		result.Success = append(result.Success, providers[k])
		for i := 0; i < len(urls); i++ {
			res := surge2SS(urls[i])
			if res.Remarks != "" {
				if len(filters) <= 0 || filters == nil {
					servers = append(servers, res)
					continue
				}
				for j := 0; j < len(filters); j++ {
					if m, _ := regexp.MatchString(filters[j], res.Remarks); m {
						servers = append(servers, res)
						break
					}
				}
			}
		}
	}
	fmt.Println(fmt.Sprintf("\n----------------\n成功获取：\n - %s\n格式错误：\n - %s\n网络错误：\n - %s\n----------------\n", strings.Join(result.Success, "\n - "), strings.Join(result.Fromat, "\n - "), strings.Join(result.Network, "\n - ")))
	gui.Configs = servers
	outputJSON, _ := json.Marshal(gui)
	writeFileErr := ioutil.WriteFile("gui-config.json", outputJSON, 0644)
	if writeFileErr == nil {
		fmt.Println(fmt.Sprintf("服务器更新完毕，合计更新%d个节点", len(servers)))
		fmt.Println("请重启Shadowsocks客户端或进入节点列表点击确定")
	} else {
		fmt.Println("配置文件写入失败")
	}
}
