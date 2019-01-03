package goini

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

var (
	defaultComment     = []byte{'#'} // 注释标记
	alternativeComment = []byte{';'} // 注释标记
	equal              = []byte{'='} // 赋值标记
	quote              = []byte{'"'} // 双引号标记
	sectionStart       = []byte{'['} // 节点开始标记
	sectionEnd         = []byte{']'} // 节点结束标记
	lineBreak          = "\n"
	empty              []byte
)

// 默认节点
const DefaultSection = "default"

type entry struct {
	section string
	key     string
	value   string
	env     string
	comment string //key的注释
}

type entries map[string]*entry

func (e entries) GetString(key string) string  {
	if v, ok := e[key]; ok {
		if isValueEnv(v.value) {
			_,vv := ParseValueEnv(v.value)
			return vv
		}
		return v.value
	}
	return ""
}

func (e entries) DefaultString(key string, val string) string {
	if v:= e.GetString(key);v != "" {
		return v
	}
	return val
}

func (e entries) DefaultStrings(key string, sep string, val []string) []string {
	if v := e.GetString(key);  v != "" {
		return strings.Split(v, sep)
	}
	return val
}
func (e entries) DefaultInt(key string, val int) int {
	if v:= e.GetString(key);v != "" {
		if vv, err := strconv.Atoi(v); err == nil {
			return vv
		}
	}
	return val
}

func (e entries) DefaultInt64(key string, val int64) int64 {
	if v:= e.GetString(key);v != "" {
		if vv, err := strconv.ParseInt(v, 10, 64); err == nil {
			return vv
		}
	}
	return val
}

func (e entries) DefaultFloat(key string, val float64) float64 {
	if v:= e.GetString(key);v != "" {
		if vv, err := strconv.ParseFloat(v, 64); err == nil {
			return vv
		}
	}
	return val
}

func (e entries) DefaultBool(key string, val bool) bool {
	if v:= e.GetString(key);v != "" {
		if vv, err := ParseBool(v); err == nil {
			return vv
		}
	}
	return val
}

type IniContainer struct {
	sync.RWMutex
	values         map[string]entries
	sectionComment map[string]string //节点的注释
	endComment     string            //文件结束注释，一般在文件尾部
}

//从文件中加载配置信息
func LoadFromFile(path string) (ini *IniContainer, err error) {
	ini, err = parseFile(path, DefaultSection)
	return
}

func NewConfig() *IniContainer {
	return &IniContainer{
		RWMutex: sync.RWMutex{},
		values:  make(map[string]entries),
	}
}

//添加一个配置节点.
// 重复的key会覆盖已存在的值
//例如:
// ini.AddEntry("","session","true")		 //添加到默认节点
// ini.AddEntry("session","session","false") //添加到自定义节点
func (c *IniContainer) AddEntry(section, key, value string) *IniContainer {
	c.RWMutex.Lock()
	defer c.RWMutex.Unlock()
	section = strings.TrimSpace(section)
	key = strings.TrimSpace(key)
	value = strings.TrimSpace(value)

	if section == "" {
		section = DefaultSection
	}

	if c.values == nil {
		c.values = make(map[string]entries)
	}
	if c.values[section] == nil {
		c.values[section] = make(map[string]*entry)
	}
	k, _ := ParseValueEnv(value)

	ev := &entry{
		section: section,
		key:     key,
		value:   value,
	}
	if k != "" {
		ev.env = k
	}
	c.values[section][key] = ev
	return c
}

//删除指定节点下的key.
//如果删除成功返回 true，如果节点不存在返回 false.
func (c *IniContainer) DeleteKey(section, key string) bool {
	c.Lock()
	defer c.Unlock()
	section = strings.TrimSpace(section)
	key = strings.TrimSpace(key)
	if section == "" {
		section = DefaultSection
	}
	if _, ok := c.values[section]; ok {
		if _, ok := c.values[section][key]; ok {
			delete(c.values[section], key)
			return true
		}
	}
	return false
}

//删除指定的节点.
//如果删除成功返回 true，如果不存在返回false
func (c *IniContainer) DeleteSection(section string) bool {
	c.Lock()
	defer c.Unlock()
	section = strings.TrimSpace(section)
	if section == "" {
		section = DefaultSection
	}
	if _, ok := c.values[section]; ok {
		delete(c.values, section)
		return true
	}
	return false
}

//添加一个节点.
func (c *IniContainer) AddSection(section string) *IniContainer {
	c.Lock()
	defer c.Unlock()
	section = strings.TrimSpace(section)
	if section == "" {
		section = DefaultSection
	}
	if _, ok := c.values[section]; !ok {
		if c.values == nil {
			c.values = make(map[string]entries)
		}
		c.values[section] = make(map[string]*entry)
	}
	return c
}

//将配置保存到文件中
func (c *IniContainer) SaveFile(path string) error {
	f, err := os.Create(path)

	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(c.String())

	return err
}

//输出字符串
func (c *IniContainer) String() string {
	body := ""

	if c == nil || len(c.values) <= 0 {
		return body
	}
	if section, ok := c.values[DefaultSection]; ok {
		if section != nil && len(section) > 0 {
			for _, vv := range section {
				if vv.comment != "" {
					body += lineBreak + vv.comment + lineBreak
				}
				body += vv.key + "=\"" + vv.value + "\"" + lineBreak
			}
		}
	}
	for k, v := range c.values {
		if k == "default" {
			continue
		}
		//如果存在节点注释
		if c, ok := c.sectionComment[k]; ok {
			body += lineBreak + c + lineBreak + "[" + k + "]" + lineBreak
		} else {
			body += lineBreak + "[" + k + "]" + lineBreak
		}

		if v != nil && len(v) > 0 {
			for _, vv := range v {
				if vv.comment != "" {
					body += lineBreak + vv.comment + lineBreak
				}
				body += vv.key + "=\"" + vv.value + "\"" + lineBreak
			}
		}
	}

	body += c.endComment
	return body
}

func parseData(data []byte, section string, dir string) (*IniContainer, error) {
	if data == nil || len(data) <= 0 {
		return &IniContainer{}, errors.New("data is empty")
	}

	cfg := &IniContainer{
		RWMutex:        sync.RWMutex{},
		values:         make(map[string]entries),
		sectionComment: make(map[string]string),
	}
	cfg.Lock()
	defer cfg.Unlock()

	buf := bufio.NewReader(bytes.NewBuffer(data))
	// check the BOM
	head, err := buf.Peek(3)
	if err == nil && head[0] == 239 && head[1] == 187 && head[2] == 191 {
		for i := 1; i <= 3; i++ {
			_, _ = buf.ReadByte()
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
			cfg.values = make(map[string]entries)
		}
		if cfg.values[section] == nil {
			cfg.values[section] = make(map[string]*entry)
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
		values:  make(map[string]entries),
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

func ParseBool(val interface{}) (value bool, err error) {
	if val != nil {
		switch v := val.(type) {
		case bool:
			return v, nil
		case string:
			switch v {
			case "1", "t", "T", "true", "TRUE", "True", "YES", "yes", "Yes", "Y", "y", "ON", "on", "On":
				return true, nil
			case "0", "f", "F", "false", "FALSE", "False", "NO", "no", "No", "N", "n", "OFF", "off", "Off":
				return false, nil
			}
		case int8, int32, int64:
			strV := fmt.Sprintf("%d", v)
			if strV == "1" {
				return true, nil
			} else if strV == "0" {
				return false, nil
			}
		case float64:
			if v == 1.0 {
				return true, nil
			} else if v == 0.0 {
				return false, nil
			}
		}
		return false, fmt.Errorf("parsing %q: invalid syntax", val)
	}
	return false, fmt.Errorf("parsing <nil>: invalid syntax")
}

func (c *IniContainer) getData(key string) string {
	if len(key) == 0 {
		return ""
	}
	c.RLock()
	defer c.RUnlock()

	var (
		section, k string
		sectionKey = strings.Split(strings.ToLower(key), "::")
	)
	if len(sectionKey) >= 2 {
		section = sectionKey[0]
		k = sectionKey[1]
	} else {
		section = DefaultSection
		k = sectionKey[0]
	}
	if v, ok := c.values[section]; ok {
		if vv, ok := v[k]; ok {
			if isValueEnv(vv.value) {
				_,vvv := ParseValueEnv(vv.value)
				return vvv
			}
			return vv.value
		}
	}
	return ""
}
func (c *IniContainer) Bool(key string) (bool, error) {
	return ParseBool(c.getData(key))
}

func (c *IniContainer) DefaultBool(key string, def bool) bool {
	v, err := c.Bool(key)
	if err != nil {
		return def
	}
	return v
}

// Int returns the integer value for a given key.
func (c *IniContainer) Int(key string) (int, error) {
	return strconv.Atoi(c.getData(key))
}

// DefaultInt returns the integer value for a given key.
// if err != nil return defaultVal
func (c *IniContainer) DefaultInt(key string, defaultVal int) int {
	v, err := c.Int(key)
	if err != nil {
		return defaultVal
	}
	return v
}

// Int64 returns the int64 value for a given key.
func (c *IniContainer) Int64(key string) (int64, error) {
	return strconv.ParseInt(c.getData(key), 10, 64)
}

// DefaultInt64 returns the int64 value for a given key.
// if err != nil return defaultVal
func (c *IniContainer) DefaultInt64(key string, defaultVal int64) int64 {
	v, err := c.Int64(key)
	if err != nil {
		return defaultVal
	}
	return v
}

// Float returns the float value for a given key.
func (c *IniContainer) Float(key string) (float64, error) {
	return strconv.ParseFloat(c.getData(key), 64)
}

// DefaultFloat returns the float64 value for a given key.
// if err != nil return defaultVal
func (c *IniContainer) DefaultFloat(key string, defaultVal float64) float64 {
	v, err := c.Float(key)
	if err != nil {
		return defaultVal
	}
	return v
}

// String returns the string value for a given key.
func (c *IniContainer) GetString(key string) string {
	return c.getData(key)
}

// DefaultString returns the string value for a given key.
// if err != nil return defaultVal
func (c *IniContainer) DefaultString(key string, defaultVal string) string {
	v := c.GetString(key)
	if v == "" {
		return defaultVal
	}
	return v
}

// Strings returns the []string value for a given key.
// Return nil if config value does not exist or is empty.
func (c *IniContainer) GetStrings(key string, sep string) []string {
	v := c.GetString(key)
	if v == "" {
		return nil
	}
	return strings.Split(v, sep)
}

// DefaultStrings returns the []string value for a given key.
// if err != nil return defaultVal
func (c *IniContainer) DefaultStrings(key string, defaultVal []string) []string {
	v := c.GetStrings(key, ";")
	if v == nil {
		return defaultVal
	}
	return v
}

// GetSection returns map for the given section
func (c *IniContainer) GetSection(section string) (map[string]string, error) {
	c.RLock()
	defer c.RUnlock()

	if v, ok := c.values[section]; ok {
		values := make(map[string]string)

		for k, vv := range v {
			values[k] = vv.value
		}
		return values, nil
	}
	return nil, errors.New("not exist section")
}

// Set writes a new value for key.
// if write to one section, the key need be "section::key".
// if the section is not existed, it panics.
func (c *IniContainer) Set(key, value string) error {
	c.Lock()
	defer c.Unlock()
	if len(key) == 0 {
		return errors.New("key is empty")
	}

	var (
		section, k string
		sectionKey = strings.Split(strings.ToLower(key), "::")
	)

	if len(sectionKey) >= 2 {
		section = sectionKey[0]
		k = sectionKey[1]
	} else {
		section = DefaultSection
		k = sectionKey[0]
	}

	if _, ok := c.values[section]; !ok {
		c.values[section] = make(map[string]*entry)
	}
	v := &entry{
		value:   value,
		key:     k,
		section: section,
	}

	c.values[section][k] = v
	return nil
}

//遍历所有 Section .
func (c *IniContainer) ForEach(fn func(section string, entries entries) bool) {
	for s, entries := range c.values {
		if !fn(s, entries) {
			return
		}
	}
}
