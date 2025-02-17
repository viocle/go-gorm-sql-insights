package insights

import (
	"reflect"
	"runtime"
	"strings"

	"gorm.io/gorm"
)

var (
	// import paths for caller filtering
	_packageImportPath = reflect.TypeOf(SQLInsights{}).PkgPath() + "."
	_gormImportPath    = reflect.TypeOf(gorm.DB{}).PkgPath() + "."
)

// callerInfo is a struct that holds information about the function that performed the DB operation
type callerInfo struct {
	Filename string
	Line     int
	Function string
}

// getCallers returns the calling functions for the current DB operation. The depth parameter specifies how many callers to collect in the stack
func getCallers(depth int) []*callerInfo {
	if depth < 1 {
		// not collecting callers
		return nil
	}
	// storage for caller function pointers, allocate min capacity of 15
	fptrs := make([]uintptr, 5+depth)
	// get callers, skip 5 levels (this function, our caller, and the gorm related functions)
	if runtime.Callers(5, fptrs) == 0 {
		// nothing there, return blank
		return nil
	}

	// loop through callers and collect function call stack details
	ret := make([]*callerInfo, 0, depth)
	for _, p := range fptrs {
		if p == 0 {
			continue
		}
		if f := runtime.FuncForPC(p); f != nil {
			// get line function name
			funcNameWithPath := f.Name()
			if !strings.HasPrefix(funcNameWithPath, _packageImportPath) &&
				!strings.HasPrefix(funcNameWithPath, _gormImportPath) {
				// outside of our package and GORM, so we can stop here
				fileName, fileLine := f.FileLine(p)
				ret = append(ret, &callerInfo{
					Filename: fileName,
					Line:     fileLine,
					Function: funcNameWithPath,
				})
				depth--
				if depth <= 0 {
					// we have collected our max callers
					return ret
				}
			}
		}
	}

	return ret
}
