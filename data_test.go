// +build !windows
package main

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"
)

var (
	hptest1 = &HaiPa{
		//实际测试时修改UserKey
		UserKey: "0bf23c28111111111111193b3a2",
		//URL带有网关
		UpdateSensorsURL: `http://www.lewei50.com/api/V1/gateway/UpdateSensors/01`,
		Timeout:          5,
		CO2InvalidValue: &CO2{
			Index: 3,
			Value: 5000,
		},
		FrequencyLimit: 20,
		Debug:          true,
		Serial: &COM{
			Name: "COM3",
			Baud: 57600,
		},
		Sensors: map[int]*V{
			1: &V{Name: "V1", Value: "1"},
			2: &V{Name: "V2", Value: "11"},
			3: &V{Name: "V3", Value: "111"},
			4: &V{Name: "V4", Value: "1111"},
			5: &V{Name: "V5", Value: "2"},
			6: &V{Name: "V6", Value: "22"},
			7: &V{Name: "V7", Value: "222"},
		},
	}

	strch = make(chan string, 10)
	errch = make(chan error, 10)
	synch = make(chan bool, 1)

	s  = "1,1,0,1,1,1,101"
	s1 = "2,2,0,2,2,2,102"
)

func createlogfile() {
	tmpdir := "d:/go/src/kongqi/kq_tmp"
	mkdir(tmpdir)
	logger := NewLogger(filepath.Join(tmpdir, "kongqi.log"))
	hptest1.logger = logger
}

func TestData(t *testing.T) {
	//fre代表服务器接受的更新最小时间
	var fre = 20
	//dur代表数据生成间隔
	var dur = 30
	//timeout代表http请求超时时间
	var timeout = hptest1.Timeout
	debug = hptest1.Debug

	createlogfile()

	go waitErr(errch, hptest1.logger)

	data1, err := NewData(hptest1.Sensors)
	if err != nil {
		t.Fatalf("newdata %s\n", err)
	}
	fmt.Printf("init data: %s\n", data1.String())

	t.Run("A", func(t *testing.T) {
		go func() {
			strch <- s
			fmt.Printf("send string to strch: %s\n", s)

			//30s
			time.Sleep(time.Second * time.Duration(dur))
			strch <- s1
			fmt.Printf("send string to strch: %s\n", s1)
		}()

		client := NewClient(timeout)

		go syncValues(data1, strch, synch, errch)
		for i := 0; i < 2; i++ {
			select {
			case <-synch:
				fmt.Printf("recv from synch %d\n", i)
				hptest1.UpdateSensors(client, data1)
				fmt.Printf("update sensors ok\n")
				//服务器限制最快更新频率设定为20s
				time.Sleep(time.Second * time.Duration(fre))
				//等待30s超时结束测试
			case <-time.After(time.Second * time.Duration(dur)):
				fmt.Println("timeout ok")
				return
			}
		}
	})

	t.Run("B", func(t *testing.T) {
		//fre代表服务器接受的更新最小时间, 测试B设置为小与限制时间间隔
		fre = 10
		//dur代表数据生成间隔
		dur = 20
		go func() {
			strch <- s
			fmt.Printf("send string to strch: %s\n", s)

			//20s
			time.Sleep(time.Second * time.Duration(dur))
			strch <- s1
			fmt.Printf("send string to strch: %s\n", s1)
		}()

		client := NewClient(timeout)

		go syncValues(data1, strch, synch, errch)
		for i := 0; i < 2; i++ {
			select {
			case <-synch:
				fmt.Printf("recv from synch %d\n", i)
				hptest1.UpdateSensors(client, data1)
				fmt.Printf("update sensors ok\n")
				//服务器限制最快更新频率设定为10s
				time.Sleep(time.Second * time.Duration(fre))
				//等待20s超时结束测试
			case <-time.After(time.Second * time.Duration(dur)):
				fmt.Println("timeout ok")
				return
			}
		}
	})
	t.Run("C", func(t *testing.T) {
		//fre代表服务器接受的更新最小时间, 测试B设置为小与限制时间间隔
		fre = 9
		//dur代表数据生成间隔
		dur = 3
		go func() {
			strch <- s
			fmt.Printf("send string to strch: %s\n", s)

			//3s
			time.Sleep(time.Second * time.Duration(dur))
			strch <- s1
			fmt.Printf("send string to strch: %s\n", s1)

			s2 := "3,3,0,3,3,3,103"
			time.Sleep(time.Second * time.Duration(dur))
			strch <- s2
			fmt.Printf("send string to strch: %s\n", s2)
		}()

		client := NewClient(timeout)

		go syncValues(data1, strch, synch, errch)
		for i := 0; i < 2; i++ {
			select {
			case <-synch:
				fmt.Printf("recv from synch %d\n", i)
				hptest1.UpdateSensors(client, data1)
				fmt.Printf("update sensors ok\n")
				//服务器限制最快更新频率设定为9s
				time.Sleep(time.Second * time.Duration(fre))
				//等待3s超时结束测试
			case <-time.After(time.Second * time.Duration(dur)):
				fmt.Println("timeout ok")
				return
			}
		}
	})
}
