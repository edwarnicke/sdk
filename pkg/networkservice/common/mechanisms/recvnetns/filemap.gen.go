// Code generated by "go-syncmap -output filemap.gen.go -type FileMap<string,*os.File>"; DO NOT EDIT.

package recvnetns

import (
	"os"
	"sync"
)

func _() {
	// An "cannot convert FileMap literal (type FileMap) to type sync.Map" compiler error signifies that the base type have changed.
	// Re-run the go-syncmap command to generate them again.
	_ = (sync.Map)(FileMap{})
}
func (m *FileMap) Store(key string, value *os.File) {
	(*sync.Map)(m).Store(key, value)
}

func (m *FileMap) LoadOrStore(key string, value *os.File) (*os.File, bool) {
	actual, loaded := (*sync.Map)(m).LoadOrStore(key, value)
	if actual == nil {
		return nil, loaded
	}
	return actual.(*os.File), loaded
}

func (m *FileMap) Load(key string) (*os.File, bool) {
	value, ok := (*sync.Map)(m).Load(key)
	if value == nil {
		return nil, ok
	}
	return value.(*os.File), ok
}

func (m *FileMap) Delete(key string) {
	(*sync.Map)(m).Delete(key)
}

func (m *FileMap) Range(f func(key string, value *os.File) bool) {
	(*sync.Map)(m).Range(func(key, value interface{}) bool {
		return f(key.(string), value.(*os.File))
	})
}
