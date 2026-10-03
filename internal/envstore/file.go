package envstore

import (
	"fmt"
	"os"
)

// ParseFile 读取受控的 shell 风格环境变量文件。
// 桌面服务只接受赋值、引号和注释，不执行命令替换或其他 shell 语法。
func ParseFile(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取环境变量文件失败: %w", err)
	}
	values, err := Parse(data)
	if err != nil {
		return nil, err
	}
	return values, nil
}

// Parse decodes a previously captured environment-file snapshot without shell
// execution, allowing callers to validate and restore the same file contents.
func Parse(data []byte) (map[string]string, error) {
	return parse(data)
}

// Marshal 将环境变量写成可被 AgentDock 安装脚本和原生运行时共同读取的格式。
func Marshal(values map[string]string) []byte {
	return marshal(values)
}
