package goini

import (
	"sync"
	"errors"
	"bufio"
	"bytes"
	"io"
	"strings"
	"path/filepath"
	"os"
	"io/ioutil"
	"log"
)

var (
	defaultSection     = "default"   // 默认节点,
	defaultComment     = []byte{'#'} // 注释标记
	alternativeComment = []byte{';'} // 注释标记
	empty              = []byte{}
	equal              = []byte{'='} // 赋值标记
	quote              = []byte{'"'} // 双引号标记
	sectionStart       = []byte{'['} // 节点开始标记
	sectionEnd         = []byte{']'} // 节点结束标记
	lineBreak          = "\n"
)


type entry struct {
	section string
	key     string
	value   string
	env     string
	comment string //key的注释
}

type IniContainer struct {
	sync.RWMutex
	values map[string]map[string]*entry
	sectionComment map[string]string //节点的注释
	endComment string //文件结束注释，一般在文件尾部
}

//从文件中加载配置信息
func LoadFromFile(path string) (ini *IniContainer, err error) {
	ini, err = parseFile(path, defaultSection)
	return
}

func NewConfig() *IniContainer  {
	return &IniContainer{
		RWMutex: sync.RWMutex{},
		values: make(map[string]map[string]*entry),
	}
}

//添加一个配置节点.
// 重复的key会覆盖已存在的值
//例如:
// ini.AddEntry("","session","true")		 //添加到默认节点
// ini.AddEntry("session","session","false") //添加到自定义节点
func (ini *IniContainer) AddEntry(section, key, value string) *IniContainer {
	ini.RWMutex.Lock()
	defer ini.RWMutex.Unlock()
	section = strings.TrimSpace(section)
	key = strings.TrimSpace(key)
	value = strings.TrimSpace(value)

	if section == "" {
		section = defaultSection
	}

	if ini.values == nil {
		ini.values = make(map[string]map[string]*entry)
	}
	if ini.values[section] == nil {
		ini.values[section] = make(map[string]*entry)
	}
	k,_ := ParseValueEnv(value)

	ev := &entry{
		section: section,
		key:     key,
		value:   value,
	}
	if k != "" {
		ev.env = k
	}
	ini.values[section][key] = ev
	return ini
}

//删除指定节点下的key.
//如果删除成功返回 true，如果节点不存在返回 false.
func (ini *IniContainer) DeleteKey(section, key string) bool {
	ini.Lock()
	defer ini.Unlock()
	section = strings.TrimSpace(section)
	key = strings.TrimSpace(key)
	if section == "" {
		section = defaultSection
	}
	if _,ok := ini.values[section]; ok {
		if _,ok := ini.values[section][key]; ok {
			delete(ini.values[section],key)
			return true
		}
	}
	return false
}

//删除指定的节点.
//如果删除成功返回 true，如果不存在返回false
func (ini *IniContainer) DeleteSection(section string) bool  {
	ini.Lock()
	defer ini.Unlock()
	section = strings.TrimSpace(section)
	if section == "" {
		section = defaultSection
	}
	if _,ok := ini.values[section]; ok {
		delete(ini.values,section)
		return true
	}
	return false
}

//添加一个节点.
func (ini *IniContainer) AddSection(section string) *IniContainer {
	ini.Lock()
	defer ini.Unlock()
	section = strings.TrimSpace(section)
	if section == "" {
		section = defaultSection
	}
	if _,ok := ini.values[section]; !ok {
		if ini.values == nil {
			ini.values = make(map[string]map[string]*entry)
		}
		ini.values[section] = make(map[string]*entry)
	}
	return ini
}

//将配置保存到文件中
func (ini *IniContainer) SaveFile(path string) error {
	f,err := os.Create(path)

	if err != nil {
		return err
	}
	defer f.Close()
	_,err = f.WriteString(ini.String())

	return err
}

//输出字符串
func (ini *IniContainer) String() string {
	body := ""

	if ini == nil || len(ini.values) <= 0 {
		return body
	}
	if section,ok := ini.values[defaultSection]; ok {
		if section != nil && len(section) > 0 {
			for _, vv := range section {
				if vv.comment != "" {
					body += lineBreak + vv.comment + lineBreak
				}
				body += vv.key + "=\"" + vv.value +"\"" + lineBreak
			}
		}
	}
	for k, v := range ini.values {
		if k == "default" {
			continue
		}
		//如果存在节点注释
		if c,ok := ini.sectionComment[k]; ok {
			body +=  lineBreak + c + lineBreak + "[" + k + "]" + lineBreak
		}else{
			body += lineBreak + "[" + k + "]" + lineBreak
		}


		if v != nil && len(v) > 0 {
			for _, vv := range v {
				if vv.comment != "" {
					body += lineBreak + vv.comment + lineBreak
				}
				body +=  vv.key + "=\"" + vv.value +"\"" + lineBreak
			}
		}
	}

	body += ini.endComment
	return body
}

func parseData(data []byte, section string, dir string) (*IniContainer, error) {
	if data == nil || len(data) <= 0 {
		return &IniContainer{}, errors.New("data is empty")
	}

	cfg := &IniContainer{
		RWMutex: sync.RWMutex{},
		values:  make(map[string]map[string]*entry, 0),
		sectionComment: make(map[string]string),
	}
	cfg.Lock()
	defer cfg.Unlock()

	buf := bufio.NewReader(bytes.NewBuffer(data))
	// check the BOM
	head, err := buf.Peek(3)
	if err == nil && head[0] == 239 && head[1] == 187 && head[2] == 191 {
		for i := 1; i <= 3; i++ {
			buf.ReadByte()
		}
	}
	var comment bytes.Buffer

	for {
		line, _, err := buf.ReadLine()

		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		line = bytes.TrimSpace(line)
		if bytes.Equal(line, empty) {
			continue
		}
		//不解析注释块
		if bytes.HasPrefix(line, defaultComment) || bytes.HasPrefix(line, alternativeComment) {
			if comment.Len() > 0 {
				comment.WriteString(lineBreak)
			}
			comment.Write(line)
			continue
		}
		//解析节点
		if bytes.HasPrefix(line, sectionStart) && bytes.HasSuffix(line, sectionEnd) {
			section = strings.ToLower(string(line[1 : len(line)-1]))
			//当解析到节点时，将注释写给当前节点
			cfg.sectionComment[section] = comment.String()
			comment.Reset()
			continue
		}
		kv := bytes.SplitN(line, equal, 2)
		//解析key
		key := strings.ToLower(string(bytes.TrimSpace(kv[0])))
		//如果存在include前缀，表明该行是包含文件
		if len(kv) == 1 && strings.HasPrefix(key, "include ") {

			includeFiles := strings.Fields(key)
			if includeFiles[0] == "include" && len(includeFiles) == 2 {

				otherFile := strings.Trim(includeFiles[1], "\"")
				if !filepath.IsAbs(otherFile) {
					otherFile = filepath.Join(dir, otherFile)
				}

				f, err := os.Stat(otherFile)
				//如果获取目标信息失败则跳过
				if err != nil {
					log.Println("file or directory does not exist: ", err, otherFile)
					continue
				}
				//如果是目录，则要扫描目录下的文件并解析
				if f.IsDir() {
					err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
						//只解析ini或conf为后缀的文件
						if !info.IsDir() && (strings.HasSuffix(info.Name(), ".ini") || strings.HasSuffix(info.Name(), ".conf")) {
							ini, err := parseFile(path, section)

							if err == nil && ini != nil {
								for k, v := range ini.values {
									if _, ok := cfg.values[k]; !ok {
										cfg.values[k] = v
									} else {
										//如果已存在顶级键，则遍历二级键
										for kk, vv := range v {
											cfg.values[k][kk] = vv
										}
									}
								}
							}
						}
						return nil
					})
					if err != nil {
						log.Println("scan directory error:", err, otherFile)
						continue
					}
				} else {
					ini, err := parseFile(otherFile, section)

					if err == nil && ini != nil {
						for k, v := range ini.values {
							if _, ok := cfg.values[k]; !ok {
								cfg.values[k] = v
							} else {
								//如果已存在顶级键，则遍历二级键
								for kk, vv := range v {
									cfg.values[k][kk] = vv
								}
							}
						}
					}
					continue
				}
			}
		}

		if len(kv) != 2 {
			return nil, errors.New("read the content error: \"" + string(line) + "\", should key = val")
		}
		val := bytes.TrimSpace(kv[1])
		if bytes.HasPrefix(val, quote) {
			val = bytes.Trim(val, `"`)
		}
		entryValue := &entry{
			section: section,
			key:     key,
			value:   string(val),
			comment: comment.String(),
		}
		comment.Reset()
		if isValueEnv(entryValue.value) {
			k, _ := ParseValueEnv(entryValue.value)
			entryValue.env = k
		}
		if cfg.values == nil {
			cfg.values = make(map[string]map[string]*entry)
		}
		if cfg.values[section] == nil {
			cfg.values[section] =  make(map[string]*entry)
		}
		cfg.values[section][key] = entryValue
	}
	if comment.Len() > 0 {
		cfg.endComment = comment.String()
	}
	return cfg, nil
}

//解析文件
func parseFile(path string, section string) (*IniContainer, error) {

	b, err := ioutil.ReadFile(path)

	if err != nil {
		log.Println("read file error: ", err, path)
		return nil, err
	}
	return parseData(b, section, filepath.Dir(path))
}

//合并配置
func Merge(config1 *IniContainer, config2 *IniContainer) *IniContainer {
	if config1 == nil && config2 == nil {
		return nil
	}
	if config2 == nil && config1 != nil {
		return config1
	}
	if config2 != nil && config1 == nil {
		return config2
	}
	cfg := &IniContainer{
		RWMutex: sync.RWMutex{},
		values:  make(map[string]map[string]*entry, 0),
	}
	cfg.Lock()
	defer cfg.Unlock()

	for k, v := range config1.values {
		cfg.values[k] = v
	}
	for k, v := range config2.values {
		if _, ok := cfg.values[k]; !ok {
			cfg.values[k] = v
		} else {
			//如果已存在顶级键，则遍历二级键
			for kk, vv := range v {
				cfg.values[k][kk] = vv
			}
		}
	}
	return cfg
}

//解析环境变量表达式.
//
// 支持以 ${ 开头 并且以 } 结尾的表达式。
//
//例如：
//	goini.ParseValueEnv("${GOPATH}")		//返回环境变量 GOPATH 和真实值
//  goini.ParseValueEnv("${GOPATHS}")
func ParseValueEnv(value string) (envKey string, realValue string) {
	if isValueEnv(value) {
		value = strings.Trim(value, "\"");
		value = value[2 : len(value)-1]
		//如果不存在 || 表示没有设置默认值
		if !strings.Contains(value, "||") {
			realValue = os.Getenv(value)
			envKey = value
			return
		}
		values := strings.SplitN(value, "||", 2)
		if len(values) == 2 {
			realValue = os.Getenv(values[0])
			if realValue == "" {
				realValue = values[1]
			}
			envKey = values[0]
			return
		} else {
			realValue = os.Getenv(values[0])
			envKey = values[0]
			return
		}

	}
	return "", value
}

//判断一个值是否是环境变量值
func isValueEnv(value string) bool {
	value = strings.Trim(value, "\"");
	return strings.HasPrefix(value, "${") && strings.HasSuffix(value, "}")
}

func (ini *IniContainer) DefaultBool(key string,def bool) bool  {

	return def
}