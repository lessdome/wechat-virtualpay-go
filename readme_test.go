package wechat_virtualpay

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// 本文件是**文档一致性测试**：README.md 与 doc.go 是手写的散文，改代码时最容易忘了改它们。
//
// 旧 README 就是这么死的：它点名的 Env、EnvSandbox、ErrorCode、NewClient、Config 如今一个
// 都不存在，而没有任何东西会因此报红——它一路漂到与代码全面矛盾，最后只能删掉重写。这几条
// 测试把「文档里写的」和「代码里的」绑在一起，让漂移在 `go test` 里就现形。
//
// 这组测试读文件系统（os.ReadFile / go/parser），是本仓库唯一这么做的测试。os 与 go/* 都是
// 标准库，不破「零第三方依赖」这条线——那条承诺本身也由这里的 TestNoThirdPartyDeps 守着。

const (
	readmePath = "README.md"
	docPath    = "doc.go"
)

// xpayCallSite 是一个服务端接口的调用点。
type xpayCallSite struct {
	path string // 如 /xpay/query_order
	tier string // PostTokenOnly / PostWithPaySig / PostWithUserSig
	file string // 定义它的非测试 .go 文件
}

// collectCallSites 扫**非测试**的 .go 文件，取出所有「函数名以 Post 开头、实参里有 /xpay/
// 字符串字面量」的调用点。
//
// 为什么走语法树而不是 grep：路径字面量夹在参数中间，正则既要认函数名又要认它是第几个参数，
// 签名一变就漏；走 ast 只认「这个调用的实参里有这么个字面量」，签名怎么改都跟得住，而且
// 顺便拿到了档位（函数名）与归属文件。
func collectCallSites(t *testing.T) []xpayCallSite {
	t.Helper()
	fset := token.NewFileSet()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("读当前目录失败: %v", err)
	}
	var sites []xpayCallSite
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatalf("解析 %s 失败: %v", name, err)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			var fnName string
			switch fn := call.Fun.(type) {
			case *ast.Ident:
				fnName = fn.Name
			case *ast.SelectorExpr:
				fnName = fn.Sel.Name
			}
			if !strings.HasPrefix(fnName, "Post") {
				return true
			}
			for _, arg := range call.Args {
				lit, ok := arg.(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					continue
				}
				p, err := strconv.Unquote(lit.Value)
				if err != nil || !strings.HasPrefix(p, "/xpay/") {
					continue
				}
				sites = append(sites, xpayCallSite{path: p, tier: fnName, file: name})
			}
			return true
		})
	}
	if len(sites) == 0 {
		t.Fatal("一个 /xpay/ 调用点都没扫到——探针坏了，别把「扫不到」当成「文档没问题」")
	}
	return sites
}

// readmeTableRow 是 README 里「服务端接口」那张分类表的一行。
type readmeTableRow struct {
	class    string
	file     string
	count    int
	tiers    map[string]int
	lineNo   int
	rawCount string
}

var (
	backtickRE = regexp.MustCompile("`([^`\n]+)`")
	// 表里「文件」一栏形如 `xpay_order.go`
	tableFileRE = regexp.MustCompile("^`([a-z0-9_]+\\.go)`$")
	// 表里「档位」一栏形如 `PostWithPaySig` ×4 + `PostTokenOnly` ×1
	tableTierRE = regexp.MustCompile("`(Post[A-Za-z]+)`\\s*×\\s*(\\d+)")
	// README / doc.go 里出现的接口路径
	xpayPathRE = regexp.MustCompile("/xpay/[a-z0-9_]+")
	// README 里指回仓库的地址（`go get` 行、CI 徽章、pkg.go.dev 链接）
	repoURLRE = regexp.MustCompile(`github\.com/[A-Za-z0-9_.\-/]+`)
	// 「整个反引号里就是一个标识符」才算要核的名字（带点、带括号、带空格的一律不管）
	identRE = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
)

// parseClassTable 解析 README 里那张分类表。
//
// 只认「表格行 + 恰好 5 栏 + 第一栏不是表头」的行；表里任何一行的格式写歪了都会在这里报错，
// 而不是被悄悄跳过——跳过的后果是那张表可以随便写而不被发现，正是这条测试要防的事。
func parseClassTable(t *testing.T, readme string) []readmeTableRow {
	t.Helper()
	var rows []readmeTableRow
	for i, line := range strings.Split(readme, "\n") {
		lineNo := i + 1
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "|") || !strings.HasSuffix(trimmed, "|") {
			continue
		}
		cells := strings.Split(strings.Trim(trimmed, "|"), "|")
		if len(cells) != 5 {
			continue
		}
		for j := range cells {
			cells[j] = strings.TrimSpace(cells[j])
		}
		if cells[0] == "类" || strings.HasPrefix(cells[0], "---") {
			continue // 表头与分隔行
		}
		fileMatch := tableFileRE.FindStringSubmatch(cells[1])
		if fileMatch == nil {
			continue // 不是分类表（这篇文档里还有别的五栏表：金额单位、应答、事件……）
		}
		count, err := strconv.Atoi(cells[2])
		if err != nil {
			t.Errorf("%s:%d 分类表的「接口数」一栏不是整数: %q", readmePath, lineNo, cells[2])
			continue
		}
		pairs := tableTierRE.FindAllStringSubmatch(cells[3], -1)
		if len(pairs) == 0 {
			t.Errorf("%s:%d 分类表的「档位」一栏没解析出 `PostXxx` ×N: %q", readmePath, lineNo, cells[3])
			continue
		}
		tiers := map[string]int{}
		for _, p := range pairs {
			n, err := strconv.Atoi(p[2])
			if err != nil {
				t.Errorf("%s:%d 档位 %s 后面的 ×N 不是整数: %q", readmePath, lineNo, p[1], p[2])
				continue
			}
			tiers[p[1]] += n
		}
		rows = append(rows, readmeTableRow{
			class: cells[0], file: fileMatch[1], count: count,
			tiers: tiers, lineNo: lineNo, rawCount: cells[2],
		})
	}
	if len(rows) == 0 {
		t.Fatalf("%s 里没解析到分类表——是把表删了，还是格式改得认不出来了？", readmePath)
	}
	return rows
}

// TestReadmeClassTableMatchesSource 核对 README 的分类表与源码里的实际调用点。
//
// 它抓的是三件事，每一件都是「加了接口忘了改文档」的不同形态：
//
//   - 表里每行的「接口数」是否等于该行「档位」栏各档之和（表内自洽）；
//   - 该文件的调用点档位多重集是否与表里声明的一致（加了接口 / 换了档位都要改表）；
//   - 表的文件集合是否等于「含 /xpay/ 调用点的非测试 .go 文件」集合（新加一类文件必须进表）。
func TestReadmeClassTableMatchesSource(t *testing.T) {
	readme := readFile(t, readmePath)
	sites := collectCallSites(t)

	byFile := map[string]map[string]int{}
	byPath := map[string]xpayCallSite{}
	for _, s := range sites {
		if byFile[s.file] == nil {
			byFile[s.file] = map[string]int{}
		}
		byFile[s.file][s.tier]++
		if prev, dup := byPath[s.path]; dup {
			t.Errorf("源码里同一个路径被两个调用点用了: %s（%s 与 %s）——分类表的计数会失真",
				s.path, prev.tier, s.tier)
		}
		byPath[s.path] = s
	}

	rows := parseClassTable(t, readme)
	byTable := map[string]readmeTableRow{}
	for _, r := range rows {
		if _, dup := byTable[r.file]; dup {
			t.Errorf("%s:%d 分类表里 %s 出现了两行", readmePath, r.lineNo, r.file)
		}
		byTable[r.file] = r

		if _, err := os.Stat(r.file); err != nil {
			t.Errorf("%s:%d 分类表点了 %s，但文件不存在", readmePath, r.lineNo, r.file)
		}
		sum := 0
		for _, n := range r.tiers {
			sum += n
		}
		if sum != r.count {
			t.Errorf("%s:%d %s：接口数写的是 %s，但档位栏加起来是 %d",
				readmePath, r.lineNo, r.file, r.rawCount, sum)
		}
		if got := byFile[r.file]; !sameCounts(got, r.tiers) {
			t.Errorf("%s:%d %s 的档位与源码不符\n  分类表: %s\n  源码里: %s",
				readmePath, r.lineNo, r.file, formatCounts(r.tiers), formatCounts(got))
		}
	}

	for file := range byFile {
		if _, ok := byTable[file]; !ok {
			t.Errorf("%s 里有 /xpay/ 调用点，但分类表里没有这一行——新加了一类接口？", file)
		}
	}
	for file := range byTable {
		if _, ok := byFile[file]; !ok {
			t.Errorf("分类表里有 %s，但它没有任何 /xpay/ 调用点——这类接口被删了？", file)
		}
	}
}

// TestDocPathsExistInSource 核 README.md 与 doc.go 里出现的每个 /xpay/ 路径都真实存在。
//
// 这是防「文档里留着已删的接口」的那一半：两面只要有一处对不上就报红。占位路径（比如
// 「换成你的接口 /xpay/xxx」）会让这条测试失败——占位请用文字描述，不要写成路径的形状。
func TestDocPathsExistInSource(t *testing.T) {
	byPath := map[string]xpayCallSite{}
	for _, s := range collectCallSites(t) {
		byPath[s.path] = s
	}
	for _, name := range []string{readmePath, docPath} {
		body := readFile(t, name)
		for i, line := range strings.Split(body, "\n") {
			for _, p := range xpayPathRE.FindAllString(line, -1) {
				if _, ok := byPath[p]; !ok {
					t.Errorf("%s:%d 写了 %s，但源码里没有这个调用点——接口删了还是路径抄错了？",
						name, i+1, p)
				}
			}
		}
	}
}

// TestReadmeRepoPathsMatchModule 核 README 里每一处指回本仓库的地址（`go get` 行、CI 徽章、
// pkg.go.dev 链接）用的都是 go.mod 里那个模块路径。
//
// 为什么要单来一条：这类字符串落在别的测试的管辖范围之外——TestReadmeNamesExist 只认大写开头
// 的 Go 标识符，`github.com/<owner>/...` 是小写、带斜杠，它一概不管（readme_test.go 文件头
// 记的就是这个盲区）。而**改名时最先被忘掉的就是 README 里那几处地址**，忘了之后读起来照旧
// 通顺，只有照抄安装命令的人会失败。本测试加进来时正好是「包名与模块路径一起改名」那次，它
// 就是为下一次改名准备的。
//
// 判定边界是「整段相等、或后面紧跟 /」，不是 HasPrefix：旧模块路径
// github.com/lessdome/wechat_virtualpay_go 恰好以新路径 github.com/lessdome/wechat_virtualpay
// 开头，光用前缀匹配会把最该抓的那条漏网之鱼放过去（这条测试加进来时正是这次改名）。
//
// 扫的是 README 里**所有** github.com 地址，不按 owner 过滤：按 owner 过滤看着聪明，实际会
// 开一个静默的洞——owner 本身抄错（github.com/别家/wechat_virtualpay）就扫不到了。将来 README
// 真要正当引用别的仓库（例如拿官方 wechatpay-go 做对比），就在这里加一条白名单并写明为什么。
func TestReadmeRepoPathsMatchModule(t *testing.T) {
	module := modulePath(t)
	readme := readFile(t, readmePath)
	seen := 0
	for i, line := range strings.Split(readme, "\n") {
		for _, p := range repoURLRE.FindAllString(line, -1) {
			seen++
			if p != module && !strings.HasPrefix(p, module+"/") {
				t.Errorf("%s:%d 指向 %s，但 go.mod 里的模块路径是 %s——改名漏了这里？",
					readmePath, i+1, p, module)
			}
		}
	}
	if seen == 0 {
		t.Fatalf("%s 里一处 github.com 地址都没扫到——探针坏了，别把「扫不到」当成「没问题」", readmePath)
	}
	if !strings.Contains(readme, "go get "+module) {
		t.Errorf("%s 里没有 `go get %s`——安装命令是新人第一个要抄的东西", readmePath, module)
	}
}

// modulePath 从 go.mod 里取出 module 行。
func modulePath(t *testing.T) string {
	t.Helper()
	for _, line := range strings.Split(readFile(t, "go.mod"), "\n") {
		rest, ok := strings.CutPrefix(strings.TrimSpace(line), "module ")
		if !ok {
			continue
		}
		if p := strings.TrimSpace(rest); p != "" {
			return p
		}
	}
	t.Fatal("go.mod 里没解析出 module 行——格式变了？")
	return ""
}

// TestReadmeNamesExist 核 README.md 与 doc.go 里反引号包着的标识符在包里真实存在。
//
// 专治旧 README 的死法：它点名的 Env、EnvSandbox、ErrorCode、NewClient、Config 今天一个都
// 不存在，而当时没有任何东西会因此报红。
//
// 判定条件写死在正则里：**整段必须是 `^[A-Za-z_][A-Za-z0-9_]*$`，且首字母大写**。
// 所以 `Content-Type`、`json.Marshal`、`xpay_order.go`、`grep -n "⚠️" *.go` 都不会被当名字；
// 而 `env`、`pay_sig`、`uri`、`appKey`、`outTradeNo`、`requestVirtualPayment` 这些**小写
// 开头**的官方术语也不在管辖范围内——它们是文档/协议里的字面名，不是 Go 名字，本包对应的
// 导出名一律是大驼峰（OutTradeNo、AppKey）。
//
// 「首字母大写才算」这条规则是**故意**的：Go 的导出名必然大写开头，要引用的包内名字也必然
// 大写开头，所以这条规则一条都不会漏；反过来，把官方术语也管起来，白名单会随着每写一段文档
// 而增长，那种名单最后必然形同虚设。
func TestReadmeNamesExist(t *testing.T) {
	// 本包之外、且**大写开头**的词。每条都要说清为什么不该是 Go 标识符。
	whitelist := map[string]string{
		"LICENSE": "仓库根下的许可证文件，不是 Go 名字",
	}
	decls := collectDeclaredNames(t)

	for _, name := range []string{readmePath, docPath} {
		body := readFile(t, name)
		for i, line := range strings.Split(body, "\n") {
			for _, m := range backtickRE.FindAllStringSubmatch(line, -1) {
				token := m[1]
				if !identRE.MatchString(token) || token[0] < 'A' || token[0] > 'Z' {
					continue
				}
				if _, ok := whitelist[token]; ok {
					continue
				}
				if _, ok := decls[token]; !ok {
					t.Errorf("%s:%d 点了 `%s`，但包里没有这个名字（函数/类型/常量/变量/字段都没有）",
						name, i+1, token)
				}
			}
		}
	}
}

// TestNoThirdPartyDeps 守着本包对外的另一条核心承诺：**零第三方依赖，只用标准库**。
//
// 这条承诺写在 README 与 doc.go 的第一段里，此前却只靠自觉：谁 import 一个第三方包、
// 跑一次 go mod tidy，go.mod 里就多出一行 require，而没有任何东西会报红——文档里那句
// 「零依赖」就悄悄变成假话了。
//
// 判定很直接：go.mod 里不该出现 require。没有去解析 go.sum——零依赖的模块压根不会有
// go.sum，它一旦出现必然是上面那行 require 的**结果**，不是独立信号。replace 也一并
// 挡掉：能 replace 的东西必然是被依赖的东西。
func TestNoThirdPartyDeps(t *testing.T) {
	body := readFile(t, "go.mod")
	for i, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "//") {
			continue
		}
		for _, kw := range []string{"require", "replace", "exclude", "retract"} {
			// 既认单行的 `require x v1`，也认块开头的 `require (`。
			if trimmed == kw || strings.HasPrefix(trimmed, kw+" ") || strings.HasPrefix(trimmed, kw+"(") {
				t.Errorf("go.mod:%d 出现了 %s（%q）——本包承诺零第三方依赖，只用标准库",
					i+1, kw, trimmed)
			}
		}
	}
}

// collectDeclaredNames 收集包内所有能被引用的名字：顶层函数、类型、常量、变量，方法，
// 以及**结构体字段与接口方法**（README 会点 `MchOrderNo`、`ErrCode` 这种字段）。
//
// 不限于导出名：README 也会提到 xpayNoEnvRequest 这种内部机制。收全比收窄安全——漏收一个
// 名字会误报，把好名字告进去；收全最多是放进一个内部名，而那本来就是文档该解释的东西。
//
// **测试文件也扫**：README 会指路 ExampleBuildPayment 这种示例，以及
// TestGoodsSignDataMatchesOfficialExample 这种钉住某个说法的测试——它们都是真实的声明，
// 而且同样会因为改名而失效，所以也得跟着核。
func collectDeclaredNames(t *testing.T) map[string]struct{} {
	t.Helper()
	fset := token.NewFileSet()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("读当前目录失败: %v", err)
	}
	names := map[string]struct{}{}
	add := func(n string) {
		if n != "" && n != "_" {
			names[n] = struct{}{}
		}
	}
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".go") {
			continue
		}
		f, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatalf("解析 %s 失败: %v", name, err)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.FuncDecl:
				add(x.Name.Name)
			case *ast.TypeSpec:
				add(x.Name.Name)
			case *ast.ValueSpec:
				for _, id := range x.Names {
					add(id.Name)
				}
			case *ast.StructType:
				for _, fld := range x.Fields.List {
					for _, id := range fld.Names {
						add(id.Name)
					}
				}
			case *ast.InterfaceType:
				for _, m := range x.Methods.List {
					for _, id := range m.Names {
						add(id.Name)
					}
				}
			}
			return true
		})
	}
	return names
}

func readFile(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(name)
	if err != nil {
		t.Fatalf("读 %s 失败: %v（这些测试必须在包目录下跑）", name, err)
	}
	return string(b)
}

func sameCounts(a, b map[string]int) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

func formatCounts(m map[string]int) string {
	if len(m) == 0 {
		return "（无）"
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s ×%d", k, m[k]))
	}
	return strings.Join(parts, " + ")
}
