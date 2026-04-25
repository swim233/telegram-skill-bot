package config

import (
	"fmt"
	"os"

	"github.com/fsnotify/fsnotify"
	"github.com/spf13/viper"
	"github.com/swim233/chat_bot/lib"
	"github.com/swim233/logger"
	"go.yaml.in/yaml/v3"
)

func InitViper() {
	viper.AddConfigPath("../../../config")
	viper.AddConfigPath("../../config")
	viper.AddConfigPath("../config")
	viper.AddConfigPath("./config")
	viper.SetConfigName("config.yaml")
	viper.SetConfigType("yaml")
	err := viper.ReadInConfig()
	if err != nil {
		logger.Errorln("Error in reading config: " + err.Error())
	}
	logger.Info("Reading config: %s", viper.ConfigFileUsed())
	viper.WatchConfig()
	viper.OnConfigChange(func(in fsnotify.Event) {
		logger.Info("config file changed")
	})
}

func ChangeConfig(cfg *lib.Config) error {
	configPath := viper.ConfigFileUsed()

	// 读取原始文件
	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("读取配置文件失败: %w", err)
	}

	// 解析为 Node 树（保留注释）
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return fmt.Errorf("解析配置文件失败: %w", err)
	}

	// 将新 config 编码为 Node
	var newNode yaml.Node
	if err := newNode.Encode(cfg); err != nil {
		return fmt.Errorf("编码新配置失败: %w", err)
	}

	// 合并（保留注释）
	if err := mergeNodes(&root, &newNode); err != nil {
		return fmt.Errorf("合并配置失败: %w", err)
	}

	return writeNode(configPath, &root)
}

// unwrapDocument 解包 DocumentNode
func unwrapDocument(node *yaml.Node) *yaml.Node {
	if node.Kind == yaml.DocumentNode && len(node.Content) > 0 {
		return node.Content[0]
	}
	return node
}

// mergeNodes 用 src 的值更新 dst，保留 dst 的注释
func mergeNodes(dst, src *yaml.Node) error {
	// 统一解包 DocumentNode
	dst = unwrapDocument(dst)
	src = unwrapDocument(src)

	if dst.Kind != src.Kind {
		return fmt.Errorf("节点类型不匹配: dst=%v src=%v", dst.Kind, src.Kind)
	}

	switch dst.Kind {
	case yaml.MappingNode:
		return mergeMappingNodes(dst, src)
	case yaml.SequenceNode:
		return mergeSequenceNodes(dst, src)
	case yaml.ScalarNode:
		dst.Value = src.Value
		dst.Tag = src.Tag
	}

	return nil
}

// mergeMappingNodes 合并 mapping 节点
func mergeMappingNodes(dst, src *yaml.Node) error {
	// 构建 src 的 key->index 映射
	srcMap := make(map[string]int)
	for i := 0; i < len(src.Content)-1; i += 2 {
		srcMap[src.Content[i].Value] = i
	}

	// 遍历 dst，用 src 对应值更新
	for i := 0; i < len(dst.Content)-1; i += 2 {
		keyNode := dst.Content[i]
		valNode := dst.Content[i+1]

		srcIdx, exists := srcMap[keyNode.Value]
		if !exists {
			continue
		}

		srcVal := src.Content[srcIdx+1]
		if err := mergeNodes(valNode, srcVal); err != nil {
			return fmt.Errorf("合并字段 [%s] 失败: %w", keyNode.Value, err)
		}
	}

	// 追加 dst 中不存在的新字段
	dstMap := make(map[string]bool)
	for i := 0; i < len(dst.Content)-1; i += 2 {
		dstMap[dst.Content[i].Value] = true
	}
	for i := 0; i < len(src.Content)-1; i += 2 {
		key := src.Content[i].Value
		if !dstMap[key] {
			dst.Content = append(dst.Content, src.Content[i], src.Content[i+1])
		}
	}

	return nil
}

// mergeSequenceNodes 合并 sequence 节点，保留注释
func mergeSequenceNodes(dst, src *yaml.Node) error {
	headComment := dst.HeadComment
	lineComment := dst.LineComment
	footComment := dst.FootComment

	dst.Content = src.Content

	dst.HeadComment = headComment
	dst.LineComment = lineComment
	dst.FootComment = footComment

	return nil
}

// writeNode 原子写入文件
func writeNode(path string, node *yaml.Node) error {
	tmpPath := path + ".tmp"

	f, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("创建临时文件失败: %w", err)
	}

	encoder := yaml.NewEncoder(f)
	encoder.SetIndent(2)

	if err := encoder.Encode(node); err != nil {
		f.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("编码失败: %w", err)
	}

	if err := encoder.Close(); err != nil {
		f.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("关闭编码器失败: %w", err)
	}

	if err := f.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("关闭文件失败: %w", err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("替换文件失败: %w", err)
	}

	return nil
}

func UnmarshalConfigTest() {
	logger.Debugln(viper.AllKeys())
	cfg := lib.Config{}
	err := viper.Unmarshal(&cfg)
	if err != nil {
		logger.Suger.Fatalf("can not unmarshal config: %s", err.Error())
	}
	logger.Info("%v", cfg)
	os.Exit(0)
}
