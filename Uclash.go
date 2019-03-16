package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"

	yaml "gopkg.in/yaml.v2"
)

// Conf file named update.json
type Conf struct {
	Providers []string `json:"providers"`
	Filter    []string `json:"filter"`
	FilterOut []string `json:"filterout"`
}

// Config file named config.yml
type Config struct {
	/*Port      int      `yaml:"port"`
	SocksProt int      `yaml:"socks-port"`
	RedirPort int      `yaml:"redir-port"`
	AllowLan  bool     `yaml:"allow-lan"`
	LogLevel  string   `yaml:"log-level"`
	Exctr     string   `yaml:"external-controller"`
	Secret    string   `yaml:"secret"`*/
	Proxy      []Proxy `yaml:"Proxy"`
	ProxyGroup Group   `yaml:"Proxy Group"`
}

//Group Clash Proxy Group
type Group struct {
	Name     string `yaml:"name"`
	Type     string
	Proxies  []string
	URL      string
	Interval int
}

//Proxy clash proxy type
type Proxy struct {
	Type     string `yaml:"type"`
	Server   string `yaml:"server"`
	Port     int
	Password string
	Cipher   string
	Name     string
	Obfs     string
	ObfsHost string `yaml:"obfs-host"`
}

// Result of convertion
type Result struct {
	Success []string
	Fromat  []string
	Network []string
}

//Main Function
func main() {
	conf := ReadConf()
	var Cconf Config
	var group Group
	providers := conf.Providers
	fmt.Println(fmt.Sprintf("成功读取到%d个托管配置，开始下载...", len(providers)))
	filters := conf.Filter
	filterouts := conf.FilterOut
	fmt.Println(fmt.Sprintf("接受关键字：%s", strings.Join(filters, " | ")))
	fmt.Println(fmt.Sprintf("不接受关键字： %s", strings.Join(filterouts, " | ")))
	var result Result
	var remotes []string //每个切片表示一个surge配置文档内容
	var wg sync.WaitGroup
	wg.Add(len(providers)) //多线程
	for i := 0; i < len(providers); i++ {
		go func(url string) { //读取从providers下载配置
			defer wg.Done()
			client := &http.Client{}
			request, err := http.NewRequest("GET", url, nil)
			request.Header.Add("User-Agent", "Surge/1166 CFNetwork/955.1.2 Darwin/18.0.0")
			if err != nil {
				fmt.Println(err)
			}
			resp, err := client.Do(request)
			if err != nil {
				fmt.Println("获取托管失败", err)
				result.Network = append(result.Network, url)
			}
			defer resp.Body.Close()
			body, err := ioutil.ReadAll(resp.Body)
			if err != nil {
				fmt.Println(err)
			}
			remotes = append(remotes, string(body))
		}(providers[i])
	}
	wg.Wait() //等待下载完成
	var clash []Proxy
	for k := 0; k < len(remotes); k++ {
		urls := SurgeFromConf(remotes[k])

		//打印托管文件中的节点信息
		/*for i := 0; i < len(urls); i++ {
			fmt.Println(urls[i])
		}*/

		//未从配置读取到节点信息,将该网址加入格式错误列表
		if urls == nil {
			result.Fromat = append(result.Fromat, providers[k])
			continue
		}
		//成功从配置读取节点信息,将该网址加入成功获取列表
		result.Success = append(result.Success, providers[k])
		//将全部节点信息转换为Proxy格式结构体
		for i := 0; i < len(urls); i++ {
			res := Surge2Clash(urls[i])
			if res.Name != "" {
				//若无过滤,直接加入全部信息
				if len(filterouts) <= 0 {
					goto BEGIN2FILTER
				}
				for out := 0; out < len(filterouts); out++ {
					if on, _ := regexp.MatchString(filterouts[out], res.Name); on {
						goto FILTEROUT
					}
				}
			BEGIN2FILTER:
				if len(filters) <= 0 {
					clash = append(clash, res)
					group.Proxies = append(group.Proxies, res.Name)
					continue
				}
				for j := 0; j < len(filters); j++ {
					if m, _ := regexp.MatchString(filters[j], res.Name); m {
						clash = append(clash, res)
						group.Proxies = append(group.Proxies, res.Name)
						break
					}
				}
			FILTEROUT:
			}
		}
	}
	fmt.Println(fmt.Sprintf("\n----------------\n成功获取：\n - %s\n格式错误：\n - %s\n网络错误：\n - %s\n----------------\n", strings.Join(result.Success, "\n - "), strings.Join(result.Fromat, "\n - "), strings.Join(result.Network, "\n - ")))
	Cconf.Proxy = clash
	group.Name = "PROXY"
	group.Interval = 300
	group.Type = "url-test"
	group.URL = "https://www.gstatic.com/generate_204"
	Cconf.ProxyGroup = group
	outputYaml, _ := yaml.Marshal(Cconf)
	clasherr := ioutil.WriteFile("config.yml", outputYaml, 0644)
	CheckErr(clasherr)
	if clasherr == nil {
		fmt.Println(fmt.Sprintf("服务器更新完毕，合计更新%d个节点", len(clash)))
		fmt.Println("请手动将配置文件导入clash配置文件")
	} else {
		fmt.Println("配置文件写入失败")
	}
}

// ReadConf read update.json
func ReadConf() Conf {
	cb, err := ioutil.ReadFile("update.json")
	if err != nil {
		fmt.Println(err)
	}
	var conf Conf
	json.Unmarshal(cb, &conf)
	return conf
}

// SurgeFromConf match surge urls
func SurgeFromConf(conf string) []string {
	re, err := regexp.Compile("\\[Proxy\\]([\\s\\S]*?)\\[Proxy Group\\]")
	if err == nil {
		submatch := re.FindSubmatch([]byte(conf))
		if len(submatch) == 2 {
			return strings.Split(string(submatch[1]), "\n")
		}
		return nil
	}
	return nil
}

//Surge2Clash Convert Surge style url to Clash format
func Surge2Clash(surge string) Proxy {
	regex, _ := regexp.Compile("(.*?)\\s*=\\s*custom,(.*?),(.*?),(.*?),(.*?),") //找到所有节点信息,滤出DIRECT和格式不规范的信息
	obfsRegex, _ := regexp.Compile("obfs-host\\s*=\\s*(.*?)(?:,|$)")
	obfsTypeRegex, _ := regexp.Compile("obfs\\s*=\\s*(.*?)(?:,|$)")
	var res Proxy
	params := regex.FindSubmatch([]byte(surge))
	if len(params) == 6 {
		res.Server = strings.TrimSpace(string(params[2]))
		res.Port, _ = strconv.Atoi(strings.TrimSpace(string(params[3])))
		res.Password = strings.TrimSpace(string(params[5]))
		res.Cipher = strings.TrimSpace(string(params[4]))
		res.Name = strings.TrimSpace(string(params[1]))
		res.Type = "ss"
		obfsType := obfsTypeRegex.FindSubmatch([]byte(surge))
		if len(obfsType) == 2 {
			res.Obfs = strings.TrimSpace(string(obfsType[1]))
			obfsParams := obfsRegex.FindSubmatch([]byte(surge))
			if len(obfsParams) == 2 {
				res.ObfsHost = strings.TrimSpace(string(obfsParams[1]))
			}
		}
	}
	return res
}

//CheckErr Print error information
func CheckErr(err error) {
	if err != nil {
		fmt.Println(err.Error())
	}
	return
}
