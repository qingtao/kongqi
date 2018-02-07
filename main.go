// 本程序用于从usb串口读取空气质量数据并上传值lewei物联网
//
// kongqi -g 生成配置文件样例
//
// kongqi -c "/usr/local/kongqi/kongqi_config.json(linux)
// 或者
// kongqi -c "d:/kongqi/kongqi_config.json"(windows)
package main

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"github.com/tarm/serial"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/cookiejar"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	cfg             = flag.String("c", "", "the path for config")
	isGen           = flag.Bool("g", false, "generate the example of config")
	defConfig       = "kongqi_config.json"
	maxTimeout      = 10
	minTimeout      = 2
	indexCO2        = 3
	invalidValueCO2 = 5000
	frequencyLimit  = 20
	debug           = false
)

type COM struct {
	Name string `json:"name"`
	Baud int    `json:"baud"`
}

type CO2 struct {
	Index int `json:"index"`
	Value int `json:"value"`
}

//海帕测试
type HaiPa struct {
	//lewei Userkey
	UserKey string `json:"Userkey"`
	//updateSensors
	UpdateSensorsURL string `json:"uploadSensors"`
	Timeout          int    `json:"timeout"`
	CO2InvalidValue  *CO2   `json:"co2_invalid_value,omitempty"`
	FrequencyLimit   int    `json:"frequency_limit,omitempty"`

	//调试
	Debug bool `json:"debug"`
	//串口配置
	Serial *COM `json:"serial_config"`

	//传感器
	Sensors map[int]*V `json:"sensors_id"`
	//日志
	logger *log.Logger
}

//取当前目录
func basedir() string {
	p, err := filepath.Abs(os.Args[0])
	if err != nil {
		log.Fatalln(err)
	}
	return filepath.Dir(p)
}

//创建指定目录
func mkdir(dir string) {
	if _, err := os.Stat(dir); err != nil {
		err = os.MkdirAll(dir, os.ModeDir|0755)
		if err != nil {
			log.Fatalln(err)
		}
	}
}

//创建日志
func NewLogger(file string) *log.Logger {
	fi, err := os.Create(file)
	if err != nil {
		log.Fatalln(err)
	}
	return log.New(fi, "", log.LstdFlags)
}

//读取配置, HaiPa需要设置logger
func ReadConfig(file string) (*HaiPa, error) {
	b, err := ioutil.ReadFile(file)
	if err != nil {
		return nil, err
	}
	var haipa HaiPa
	if err = json.Unmarshal(b, &haipa); err != nil {
		return nil, err
	}
	return &haipa, nil
}

//取消自动跳转
var skipRedirect = errors.New(`stop redirect`)

func skipRedirects(req *http.Request, via []*http.Request) error {
	if len(via) > 0 {
		return skipRedirect
	}
	return nil
}

//创建新http客户端
func NewClient(timeout int) *http.Client {
	var tr http.RoundTripper = &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
	}
	var Jar, _ = cookiejar.New(nil)
	client := &http.Client{
		Transport:     tr,
		CheckRedirect: skipRedirects,
		Timeout:       time.Duration(timeout) * time.Second,
		Jar:           Jar,
	}
	return client
}

//服务端返回结果
type Result struct {
	Successful bool   `json:"Successful"`
	Message    string `json:"Message"`
}

//上传数据格式
type V struct {
	Name  string `json:"Name"`
	Value string `json:"Value,omitempty"`
}

type Data struct {
	sync.RWMutex
	Values []*V
}

func (data *Data) String() string {
	s := ""
	for i := 0; i < len(data.Values); i++ {
		v := data.Values[i]
		s += v.Name + ":" + v.Value
		if i != len(data.Values)-1 {
			s += ","
		}
	}
	return s
}

//以HaiPa.Sensors初始化Data
func NewData(sensors map[int]*V) (*Data, error) {
	//fmt.Printf("%s\n", sensors)
	var data = new(Data)
	var values = make([]*V, 0)
	for i := 0; i < len(sensors); i++ {
		values = append(values, sensors[i+1])
		//fmt.Printf("%d--%s\n", i, values)
	}
	data.Values = values
	return data, nil
}

//上传数据
func (hp *HaiPa) UpdateSensors(client *http.Client, data *Data) {
	data.RLock()
	b, err := json.MarshalIndent(data.Values, "", "  ")
	if err != nil {
		hp.logger.Printf("ERROR: json marshal values failed: %s\n", err)
		return
	}
	data.RUnlock()

	if debug {
		hp.logger.Printf("DEBUG: send values:\n----------\n%s\n----------\n", b)
	}

	r := bytes.NewReader(b)
	req, err := http.NewRequest("POST", hp.UpdateSensorsURL, r)
	if err != nil {
		hp.logger.Printf("ERROR: http post failed: %s\n", err)
		return
	}
	req.Header["userkey"] = []string{hp.UserKey}

	resp, err := client.Do(req)
	if err != nil {
		hp.logger.Printf("WARN: client do request failed: %s\n", err)
		return
	}
	defer resp.Body.Close()
	
	b, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		hp.logger.Printf("ERROR: read result failed: %s\n", err)
		return
	}

	var res Result
	if err := json.Unmarshal(b, &res); err != nil {
		hp.logger.Printf("ERROR: json unmarshal result failed: %s\n", err)
		return
	}
	if debug {
		hp.logger.Printf("DEBUG: server reply:\n----------\n%s\n----------\n", b)
	} else if !res.Successful {
		hp.logger.Printf("WARN: send values failed, the message: %s\n", res.Message)
	}
}

//打开串口
func (hp *HaiPa) OpenPort() (io.ReadWriteCloser, error) {
	//串口配置
	c := &serial.Config{Name: hp.Serial.Name, Baud: hp.Serial.Baud}
	//调用serial.OpenPort打开串口
	com, err := serial.OpenPort(c)
	if err != nil {
		hp.logger.Printf("ERROR: open serial failed: %s\n", err)
		return nil, err
	}
	return com, nil
}

//循环按行读取数据
func ReadLine(com io.Reader, sch chan<- string, ech chan<- error) {
	scanner := bufio.NewScanner(com)

	scanner.Split(bufio.ScanLines)
	for {
		for scanner.Scan() {
			sch <- scanner.Text()
		}
		if err := scanner.Err(); err != nil {
			ech <- err
		}
	}
}

//分割字符串, 获取value值
func syncValues(data *Data, sch <-chan string, synch chan<- bool, ech chan<- error) {
	length := len(data.Values)
	for s := range sch {
		vs := strings.Split(s, ",")
		//如果length小于7，继续发送错误并继续同步下一条
		if len(vs) < length {
			ech <- errors.New(fmt.Sprintf("ERROR: the values %s read from sensor length less then %d", s, length))
			continue
		}
		//fmt.Printf("testing vs %s\n", vs)
		data.Lock()
		//更新data的Values
		for i := 0; i < length; i++ {
			if i == indexCO2 {
				co2, err := strconv.ParseInt(vs[i], 10, 0)
				if err != nil {
					ech <- errors.New(fmt.Sprintf("ERROR: %s", err))
					continue
				} else if co2 > int64(invalidValueCO2) {
					ech <- errors.New(fmt.Sprintf("ERROR: the value of CO2 now: %d, invalid", co2))
					continue
				}
			}
			data.Values[i].Value = vs[i]
		}
		data.Unlock()
		select {
		//向sysch写入通知
		case synch <- true:
		//如果synch已阻塞则执行default跳过,保证按最快频率更新的数据为最新
		default:
			if debug {
				ech <- errors.New("DEBUG: sync blocking")
			}
		}
	}
}

//读取error并写入日志
func waitErr(ech <-chan error, logger *log.Logger) {
	for err := range ech {
		logger.Printf("%s\n", err)
	}
}

//生成配置文件例子
func generateConfig(file string) error {
	var hpExample = &HaiPa{
		UserKey: "01111111111111118fb1011111111111",
		//URL带有网关
		UpdateSensorsURL: `http://www.lewei50.com/api/V1/gateway/UpdateSensors/01`,
		Timeout:          5,
		CO2InvalidValue: &CO2{
			Index: 3,
			Value: 5000,
		},
		FrequencyLimit: 20,
		Debug:          false,
		Serial: &COM{
			Name: "COM3",
			Baud: 57600,
		},
		Sensors: map[int]*V{
			1: &V{Name: "V1"},
			2: &V{Name: "V2"},
			3: &V{Name: "V3"},
			4: &V{Name: "V4"},
			5: &V{Name: "V5"},
			6: &V{Name: "V6"},
			7: &V{Name: "V7"},
		},
	}

	b, err := json.MarshalIndent(hpExample, "", "  ")
	if err != nil {
		return err
	}
	if err := ioutil.WriteFile(file, b, 0660); err != nil {
		return err
	}
	fmt.Printf("generate example of config: %s\n", file)
	return nil
}
func main() {
	flag.Parse()
	base := basedir()

	if flag.NFlag() > 1 {
		flag.PrintDefaults()
		return
	}
	if *isGen {
		s := "kongqi_config_example.json"
		example := filepath.Join(base, s)
		if err := generateConfig(example); err != nil {
			log.Fatalln(err)
		}
		fmt.Printf("Note: after edit, rename %s to %s\n", s, defConfig)
		return
	}

	//检查参数并更新相关选项
	//如果配置文件路径为空，查找当前目录的kongqi_config.json
	if *cfg == "" {
		*cfg = filepath.Join(base, defConfig)
	}

	hp, err := ReadConfig(*cfg)
	if err != nil {
		log.Fatalln(err)
	}

	//如果hp.Timeout大于minTimeout,并小于maxTimeout,则将hp.Timeout赋值给maxTimeout
	if hp.Timeout > minTimeout && hp.Timeout < maxTimeout {
		maxTimeout = hp.Timeout
	}

	//服务器限制更新频率
	if hp.FrequencyLimit > frequencyLimit {
		frequencyLimit = hp.FrequencyLimit
	}

	debug = hp.Debug

	//检查传感器数量
	if length := len(hp.Sensors); length > 0 {
		co2 := hp.CO2InvalidValue
		//设置过滤二氧化碳的位置和数值
		if co2.Index >= 0 && co2.Index < length {
			indexCO2 = co2.Index
		}
		if co2.Value > 0 {
			invalidValueCO2 = co2.Value
		}
	} else {
		log.Fatalf("please set sensors")
	}

	//创建临时目录
	tmpdir := filepath.Join(base, "kq_tmp")
	mkdir(tmpdir)
	logger := NewLogger(filepath.Join(tmpdir, "kongqi.log"))
	hp.logger = logger

	var com io.ReadWriteCloser
	for {
		com, err = hp.OpenPort()
		if err != nil {
			logger.Println("wait for 90s to retry")
			time.Sleep(90 * time.Second)
		} else {
			logger.Printf("open serial device success: %s\n", hp.Serial.Name)
			break
		}
	}
	defer com.Close()

	data, err := NewData(hp.Sensors)
	if err != nil {
		log.Fatalln(err)
	}

	//创建string channel,缓存100
	var strch = make(chan string, 100)
	//创建error channel,缓存100
	var errch = make(chan error, 100)
	go waitErr(errch, logger)
	//创建一个channel接收syncValues信号
	client := NewClient(maxTimeout)

	//循环检查synch,如果接收到bool值,就执行hp.UpdateSensors,将values上传至服务端
	var synch = make(chan bool)
	//最后的接收者
	go func() {
		for range synch {
			if debug {
				logger.Printf("DEBUG: data newer received")
			}
			hp.UpdateSensors(client, data)
			time.Sleep(time.Duration(frequencyLimit) * time.Second)
		}
	}()

	//中间的接收者和发送者
	go syncValues(data, strch, synch, errch)

	//最早的发送者
	ReadLine(com, strch, errch)
}
