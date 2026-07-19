<p align="center">
	<img src="assets/logo.png" height="100" border="0" alt="RQL">
	<br/>
	<a href="https://godoc.org/github.com/a8m/rql">
		<img src="https://img.shields.io/badge/api-reference-blue.svg?style=flat-square" alt="GoDoc">
	</a>
	<a href="LICENSE">
		<img src="https://img.shields.io/badge/license-MIT-blue.svg?style=flat-square" alt="LICENSE">
	</a>
	<a href="https://app.circleci.com/pipelines/github/a8m/rql?branch=master">
		<img src="https://img.shields.io/circleci/build/github/a8m/rql?style=flat-square" alt="Build Status">
	</a>
</p>

RQL 是一个面向 REST 的资源查询语言。它为基于 SQL 数据库的 Web 应用提供了一套简单轻量的 API，用于添加动态查询能力。它充当 HTTP handler 与数据库引擎之间的连接器，统一处理用户输入的校验与翻译。

<p align="center">
  <img src="assets/diagram.png" alt="rql diagram">
</p>

## 动机

过去几年里，我在 Go 中开发过各种各样的 Web 应用，有的体量很小，有的有大量实体和复杂关系。但无论哪种情况，我都没能找到一个简单且标准的 API 来查询资源。

什么叫查询？假设我们的应用有一张 `orders` 表，我们希望用户能够按动态参数搜索和 __过滤__。例如：_查询今天所有价格大于 100 的订单_。
为此我曾把参数放在 query string 里传递，像这样：`created_at_gt=X&price_gt=100`。
但当我需要在两个条件之间做"或"运算时就会变得很复杂。比如 _查询所有已取消、或上周创建但还未发货的订单_，对应的 SQL 是：
```sql
SELECT * FROM ORDER WHERE canceled = 1 OR (created_at < X AND created_at > Y AND shipped = 0)
```
我对 MongoDB 的查询语法比较熟悉，觉得它简单且足够稳健，能满足我的需求，于是决定把它作为本项目的查询语言。我希望这个项目与具体业务无关——不依赖任何特定的应用或资源。因此，要将 rql 集成到新项目，只需 import 这个包，并在自己的 struct 定义中加上所需 tag 即可。详见 [快速开始](#快速开始) 一节。

## 快速开始

rql 使用 MongoDB 查询语法的一个子集。如果你熟悉 MongoDB 语法，上手会非常容易；即便不熟悉，它也足够简单。
要嵌入 rql，只需在你 struct 定义中加上需要的 tag（`filter` 或 `sort`），rql 会自动完成所有校验，并在查询不符合 schema 时返回清晰的错误信息给终端用户。
下面是一个快速上手的示例，更完整的文档请看 [API](#api) 一节。
```go
// 一个使用 gorm 的 HTTP handler 示例，支持从 body 或 URL query string 接收用户查询。
package main

var (
	db *gorm.DB
	// QueryParam 是 query string 中的 key 名称。
	QueryParam = "query"
	// MustNewParser 在配置非法时会 panic。
	QueryParser = rql.MustNewParser(rql.Config{
		Model:    User{},
		FieldSep: ".",
	})
)

// User 是 gorm 中的 model 定义。
type User struct {
	ID          uint      `gorm:"primary_key" rql:"filter,sort"`
	Admin       bool      `rql:"filter"`
	Name        string    `rql:"filter"`
	AddressName string    `rql:"filter"`
	CreatedAt   time.Time `rql:"filter,sort"`
}


// GetUsers 是一个 http.Handler，从 body 或 query string 接收 db 查询。
func GetUsers(w http.ResponseWriter, r *http.Request) {
	var users []User
	p, err := getDBQuery(r)
	if err != nil {
		io.WriteString(w, err.Error())
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	err = db.Where(p.FilterExp, p.FilterArgs).
		Offset(p.Offset).
		Limit(p.Limit).
		Order(p.Sort).
		Find(&users).Error
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if err := json.NewEncoder(w).Encode(users); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
}

// getDBQuery 从 body 或 query string 中提取查询 JSON，并交给 parser 解析。
func getDBQuery(r *http.Request) (*rql.Params, error) {
	var (
		b   []byte
		err error
	)
	if v := r.URL.Query().Get(QueryParam); v != "" {
		b, err = base64.StdEncoding.DecodeString(v)
	} else {
		b, err = io.ReadAll(io.LimitReader(r.Body, 1<<12))
	}
	if err != nil {
		return nil, err
	}
	return QueryParser.Parse(b)
}
```
完整可运行示例见 [examples/simple](examples/simple.go)。


## API

使用 rql 前，需要先配置 parser。我们来看一个基础示例。更详细的最新文档请查阅 [godoc](https://godoc.org/github.com/a8m/rql/#Config)。
构建 parser 有两种方式：`rql.New(rql.Config)` 和 `rql.MustNew(rql.Config)`。两者唯一差别是：`rql.New` 在配置非法时返回 error，而 `rql.MustNew` 会 panic。
```go
// 使用 rql.MustPanic 是因为顶层声明中不希望处理 error。
var Parser = rql.MustNew(rql.Config{
	// User 是我们要查询的资源。
	Model: User{},
	// ColumnFn 将 struct 字段名翻译为数据库列名。
	// 默认的 rql.Column 函数会把 CamelCase 转换为 snake_case。
	// 对于 gorm v2，默认函数即可直接使用。
	// ColumnFn: gorm.ToDBName,  // gorm v1（已弃用）
	// 使用自定义 logger。该 logger 仅在构建阶段使用。
	Log: logrus.Printf,
	// 当用户未提供 limit 时，Parse 返回的默认 limit。
	DefaultLimit: 100,
	// 只接受 limit 值大于等于 200 的请求。
	LimitMaxValue: 200,
})
```
rql 在构建阶段通过反射检测每个字段的类型，并为每个字段生成一组校验规则。如果某条规则校验失败，或 rql 遇到未知字段，会返回一个清晰的错误给用户。反射只在构建 parser 时执行一次，运行时无反射开销。
下面是校验规则列表：
1. `int`（8/16/32/64）、`sql.NullInt64` - 整数
2. `uint`（8/16/32/64）、`uintptr` - 整数且大于等于 0
3. `float`（32/64）、`sql.NullFloat64` - 数字
4. `bool`、`sql.NullBool` - 布尔值
5. `string`、`sql.NullString` - 字符串
6. `time.Time` 及其他可转换为 `time.Time` 的类型 - 默认使用 `time.RFC3339`（JS 格式），需要可被 `time.Parse` 解析。
   可以通过 tag 覆盖 `time.Time` 的 layout 格式。既可以用 `time` 包中的标准 layout，也可以自定义。例如：
   ```go
   type User struct {
		T1 time.Time `rql:"filter"`                         // time.RFC3339
		T2 time.Time `rql:"filter,layout=UnixDate"`         // time.UnixDate
		T3 time.Time `rql:"filter,layout=2006-01-02 15:04"` // 2006-01-02 15:04（自定义）
   }
   ```

注意：所有规则对指针类型同样适用。也就是说，如果你的字段是 `Name *string`，rql 仍会使用字符串校验规则。

### 用户 API

我们将开发者（通常是前端开发）视为本 API 的用户。下面介绍我们对外暴露的 JSON API。
顶层查询 JSON 接受 4 个字段：`offset`、`limit`、`filter`、`sort`，全部可选。

#### `offset` 和 `limit`

这两个字段用于分页，等价于标准 SQL 中的 `OFFSET` 和 `LIMIT`。
- `offset` 必须大于等于 0，默认为 0
- `limit` 必须大于 0 且小于等于配置项 `LimitMaxValue`。
   `LimitMaxValue` 默认值为 100

#### `sort`

`sort` 接受字符串数组（`[]string`），翻译为 SQL 的 `ORDER BY` 子句。数组中每项必须是可排序字段（即 struct tag 中带 `rql:"sort"`）。SQL 默认是升序，但可以通过可选前缀 `+` 或 `-` 控制：`+` 表示升序，`-` 表示降序。看个简短示例：
```
输入 - ["address.name", "-address.zip.code", "+age"]
结果 - address_name, address_zip_code DESC, age ASC
```

#### `select`

`select` 接受字符串数组（`[]string`），以逗号（","）拼接成 SQL `SELECT` 子句。
```
输入 - ["name", "age"]
结果 - "name, age"
```

#### `filter`

`filter` 会被翻译为 SQL 的 `WHERE` 子句。它是一个对象，包含可过滤字段或 `$or` 操作符。对象中每个字段对应 `WHERE` 子句中的一个条件，可以是与字段类型匹配的值，也可以是一个操作符对象。下面分别说明：
- 字段格式 `field: <value>`，表示使用 `=` 谓词。例如：
  ```
  输入：
  {
    "admin": true
  }

  结果：admin = ?
  ```
  可以看到，RQL 在生成的 `WHERE` 中使用了占位符。具体使用方式见 [示例](#示例) 一节。
- 字段格式 `field: { <predicate>: <value>, ...}`，例如：
  ```
  输入：
  {
    "age": {
      "$gt": 20,
      "$lt": 30
    }
  }

  结果：age > ? AND age < ?
  ```
  两个谓词之间会使用逻辑 `AND`。
  完整的谓词列表见下文。
- `$or` 是表示逻辑 `OR` 的字段，可以出现在查询的任意层级。它的值必须是条件对象数组，结果是这些对象的"或"运算。例如：
  ```
  输入：
  {
    "$or": [
      { "city": "TLV" },
      { "zip": { "$gte": 49800, "$lte": 57080 } }
    ]
  }

  结果：city = ? OR (zip >= ? AND zip <= ?)
  ```
简化记忆规则：**对象用 `AND`，数组用 `OR`**。下面列出所有支持的谓词，然后再看几个示例。

##### 谓词（Predicates）

- `$eq` 和 `$neq` - 所有类型可用
- `$gt`、`$lt`、`$gte`、`$lte` - 数字、字符串、时间戳可用
- `$like` 和 `$nlike` - 仅字符串类型可用
- `$ilike` 和 `$nilike` - 大小写不敏感的 `LIKE`/`NOT LIKE`。仅 PostgreSQL 原生支持；
  当配置了匹配的 `Dialect` 时，在 MySQL/SQLite 上会自动翻译为 `LOWER(col) LIKE LOWER(?)` / `LOWER(col) NOT LIKE LOWER(?)`。
  若未配置 `Dialect`，会输出原始的 `ILIKE`/`NOT ILIKE` SQL（在这些数据库上会执行失败）。
- `$regex` - 正则匹配。SQL 关键字取决于 `Dialect`：MySQL/SQLite 上为 `REGEXP ?`
  （SQLite 需加载 `regexp` 扩展），PostgreSQL 上为 `~ ?`。未配置 `Dialect` 时输出 `REGEXP ?`。
  仅适用于字符串字段。
- `$in` 和 `$nin` - `IN` / `NOT IN` 值列表，所有类型可用
- `$between` 和 `$nbetween` - `BETWEEN` / `NOT BETWEEN` 一对边界，数字和时间戳可用
- `$isnull` 和 `$isnotnull` - `IS NULL` / `IS NOT NULL`，所有类型可用。值会被忽略
  （例如 `{"name": {"$isnull": true}}`）。
- `$not` - 对子条件对象取反。将内部条件用 `NOT (...)` 包裹。
  例如：
  ```
  输入：
  {
    "$not": { "age": { "$gt": 10 }, "name": "foo" }
  }

  结果：NOT (age > ? AND name = ?)
  ```
- `$nor` - 对条件对象数组取"或非"。等价于 `NOT (... OR ...)`。
  例如：
  ```
  输入：
  {
    "$nor": [
      { "age": 10 },
      { "age": 20 }
    ]
  }

  结果：NOT (age = ? OR age = ?)
  ```

如果用户对字段使用了不支持的谓词，会得到清晰的错误。例如：
```
输入：
{
  "age": {
    "$like": "%0"
  }
}

结果：can not apply op "$like" on field "age"
```

## 示例

下面所有示例都假设 parser 定义如下：
```go

var QueryParser = rql.MustNewParser(rql.Config{
	Model:    	User{},
	FieldSep: 	".",
	LimitMaxValue:	25,
})
	
type User struct {
	ID          uint      `gorm:"primary_key" rql:"filter,sort"`
	Admin       bool      `rql:"filter"`
	Name        string    `rql:"filter"`
	Address     string    `rql:"filter"`
	CreatedAt   time.Time `rql:"filter,sort"`
}
```
#### 简单示例
```go
params, err := QueryParser.Parse([]byte(`{
  "limit": 25,
  "offset": 0,
  "filter": {
    "admin": false
  }
  "sort": ["+name"]
}`))
must(err, "parse should pass")
fmt.Println(params.Limit)	// 25
fmt.Println(params.Offset)	// 0
fmt.Println(params.Sort)	// "name ASC"
fmt.Println(params.FilterExp)	// "name = ?"
fmt.Println(params.FilterArgs)	// [true]
```

这里你会得到一个合法的 `rql.Param` 对象，可以把它传给你喜欢的 ORM 或查询连接器。

```go
var users []*User

// entgo.io（类型安全的 entity 框架）
users, err = client.User.Query().
    Where(func(s *sql.Selector) {
        s.Where(sql.ExprP(p.FilterExp, p.FilterArgs...))
    }).
    Limit(p.Limit).
    Offset(p.Offset).
    All(ctx)
must(err, "failed to query ent")

// gorm
err = db.Where(p.FilterExp, p.FilterArgs).
	Offset(p.Offset).
	Limit(p.Limit).
	Order(p.Sort).
	Find(&users).Error
must(err, "failed to query gorm")

// xorm
err = engine.Where(p.FilterExp, p.FilterArgs...).
	Limit(p.Limit, p.Offset).
	OrderBy(p.Sort).
	Find(&users)
must(err, "failed to query xorm")

// go-pg/pg
err = db.Model(&users).
	Where(p.FilterExp, p.FilterArgs).
	Offset(p.Offest).
	Limit(p.Limit).
	Order(p.Sort).
	Select()
must(err, "failed to query pg/orm")

// 还有更多示例？欢迎补充。
```

#### 中级示例
```go
params, err := QueryParser.Parse([]byte(`{
  "limit": 25,
  "filter": {
    "admin": false,
    "created_at": {
      "$gt": "2018-01-01T16:00:00.000Z",
      "$lt": "2018-04-01T16:00:00.000Z"
    }
    "$or": [
      { "address": "TLV" },
      { "address": "NYC" }
    ]
  }
  "sort": ["-created_at"]
}`))
must(err, "parse should pass")
fmt.Println(params.Limit)	// 25
fmt.Println(params.Offset)	// 0
fmt.Println(params.Sort)	// "created_at DESC"
fmt.Println(params.FilterExp)	// "admin = ? AND created_at > ? AND created_at < ? AND (address = ? OR address = ?)"
fmt.Println(params.FilterArgs)	// [true, Time(2018-01-01T16:00:00.000Z), Time(2018-04-01T16:00:00.000Z), "TLV", "NYC"]
```


## 数据库方言（Dialect）

`Dialect` 配置项控制方言相关操作符（`$ilike`/`$nilike`/`$regex`）如何翻译为 SQL。请将其设置为与生产数据库一致：

```go
var QueryParser = rql.MustNewParser(rql.Config{
    Model:   User{},
    Dialect: rql.DialectPostgreSQL, // 或 DialectMySQL / DialectSQLite
})
```

| 方言                | `$ilike` / `$nilike`             | `$regex`              |
|---------------------|-----------------------------------|-----------------------|
| `DialectPostgreSQL` | `col ILIKE ?` / `col NOT ILIKE ?`| `col ~ ?`             |
| `DialectMySQL`      | `LOWER(col) LIKE LOWER(?)`        | `col REGEXP ?`        |
| `DialectSQLite`     | `LOWER(col) LIKE LOWER(?)`         | `col REGEXP ?`（需加载 `regexp` 扩展） |
| `""`（空）          | 原始 `ILIKE` / `NOT ILIKE`         | 原始 `REGEXP`          |

当 `Dialect` 为空时，rql 会输出原始关键字（`ILIKE`/`REGEXP`），由数据库自行决定是否拒绝。**建议：显式设置 `Dialect` 与生产数据库匹配。**

非法方言值（例如 `Dialect: "oracle"`）会让 `NewParser` 返回错误。

## Group By、Having 与 Distinct

rql 通过三个可选的顶层字段支持 `GROUP BY`、`HAVING` 和 `SELECT DISTINCT`：

```json
{
    "select": ["name"],
    "distinct": true,
    "filter": { "age": { "$gt": 18 } },
    "group": ["age"],
    "having": { "COUNT(*)": { "$gte": 5 } }
}
```

输出的 `Params` 暴露如下字段：

| 字段         | 描述                                    | 示例                  |
|--------------|-----------------------------------------|-----------------------|
| `GroupExp`   | `GROUP BY` 的逗号分隔列名               | `"age"`               |
| `HavingExp`  | `HAVING` 的 SQL 表达式（含 `?` 占位符） | `"COUNT(*) >= ?"`     |
| `HavingArgs` | `HavingExp` 对应的参数值                | `[5]`                 |
| `Distinct`   | 为 true 时调用方应输出 `SELECT DISTINCT` | `true`                |

要在 `having` 中引用聚合表达式，需通过 `column=` tag 选项声明一个专用字段：

```go
type UserStat struct {
    Age   int    `rql:"filter"`
    Count int    `rql:"filter,column=COUNT(*)"`
}
```

然后在 `having` 对象中用 `"COUNT(*)"` 作为 key。上面的示例会生成 `HAVING COUNT(*) >= ?`，参数为 `5`。

## 后续计划与贡献

如果你想参与本项目的开发，下面是一些待办事项：
- [ ] 用于构建查询的 JS 库
- [ ] 通过特定 tag 跳过校验的选项
- [ ] 自动（或通过配置）过滤和排序 `gorm.Model` 字段
- [ ] 为 PR 添加 benchcmp
- [ ] 支持 MongoDB。输出需为 bson 对象。这里有一个 [使用示例](https://gist.github.com/congjf/8035830)
- [ ] 目前 rql 假设所有字段都被打平存储在表中，即便是嵌套字段也是如此。
  例如，给定如下 struct：
  ```go
  type User struct {
      Address struct {
          Name string `rql:"filter"`
      }
  }
  ```
  rql 假设表中存在 `address_name` 字段。但有时并非如此，`address` 可能存储在另一张表里。因此我希望为字段增加 `table=` 选项，并支持嵌套查询。
- [ ] 代码生成版本 - 低优先级

## 性能与可靠性

RQL 的性能表现相当不错，但总有优化空间。当前基准测试结果如下：

|      __测试__       | __Time/op__    | __B/op__   | __allocs/op__  |
|---------------------|----------------|------------|----------------|
| Small               |    1809        |   960      |   19           |
| Medium              |    6030        |   3100     |   64           |
| Large               |    14726       |   7625     |   148          |

我使用 `go-fuzz` 做过模糊测试，没有发现任何崩溃。欢迎你自行运行测试，找出潜在问题。

## LICENSE

本仓库代码基于 MIT 协议开源。由于这是我的个人仓库，你获得的我代码的授权来自我本人，而非我的雇主（Facebook）。
