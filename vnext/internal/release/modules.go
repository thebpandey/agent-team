package release

import (
	"bytes"
	"debug/buildinfo"
	"encoding/base64"
	"net/url"
	"reflect"
	"runtime/debug"
	"sort"
	"strings"
)

// GoModule records the executable's embedded build metadata, including an
// explicit replacement when present. Sum is Go's h1 module sum, not a file hash.
type GoModule struct {
	Path    string    `json:"path"`
	Version string    `json:"version,omitempty"`
	Sum     string    `json:"sum,omitempty"`
	Replace *GoModule `json:"replace,omitempty"`
}

func executableGoModules(body []byte) []GoModule {
	info, err := buildinfo.Read(bytes.NewReader(body))
	if err != nil {
		// Historical dependency-free manifests and non-executable test fixtures
		// retain their exact serialization. Never infer dependencies from go.mod.
		return nil
	}
	var modules []GoModule
	for _, dependency := range info.Deps {
		module := recordedGoModule(dependency)
		if dependency.Replace != nil {
			replacement := recordedGoModule(dependency.Replace)
			module.Replace = &replacement
		}
		modules = append(modules, module)
	}
	sort.Slice(modules, func(i, j int) bool { return modules[i].Path < modules[j].Path })
	return modules
}

func recordedGoModule(module *debug.Module) GoModule {
	return GoModule{Path: module.Path, Version: module.Version, Sum: module.Sum}
}

func validGoModules(modules []GoModule) bool {
	previous := ""
	for _, module := range modules {
		if module.Path <= previous || !validGoModule(module) {
			return false
		}
		previous = module.Path
	}
	return true
}

func validGoModule(module GoModule) bool {
	if module.Path == "" || strings.ContainsAny(module.Path+module.Version, "\r\n\t ") {
		return false
	}
	if module.Sum != "" {
		if !strings.HasPrefix(module.Sum, "h1:") {
			return false
		}
		digest, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(module.Sum, "h1:"))
		if err != nil || len(digest) != 32 || "h1:"+base64.StdEncoding.EncodeToString(digest) != module.Sum {
			return false
		}
	}
	return module.Replace == nil || module.Replace.Replace == nil && validGoModule(*module.Replace)
}

func executableModulesMatch(body []byte, manifest Manifest) bool {
	return reflect.DeepEqual(executableGoModules(body), manifest.GoModules)
}

func moduleComponent(module GoModule) SBOMComponent {
	var properties []SBOMProperty
	original := module
	if module.Replace != nil {
		module = *module.Replace
		properties = append(properties, SBOMProperty{Name: "go:module:requested", Value: original.Path + "@" + original.Version})
	}
	if module.Sum != "" {
		properties = append(properties, SBOMProperty{Name: "go:module:sum", Value: module.Sum})
	}
	component := SBOMComponent{Name: module.Path, Version: module.Version, Type: "library", BOMRef: "go-module:" + url.PathEscape(original.Path), Properties: properties}
	if module.Version != "" {
		component.PURL = "pkg:golang/" + strings.ReplaceAll(url.PathEscape(module.Path), "%2F", "/") + "@" + url.PathEscape(module.Version)
		component.BOMRef = component.PURL
	}
	return component
}
