# ctx JSON API

`ctx` は、Add-on や外部ツールから保存済みデータを利用するための JSON 出力を提供します。

この API は HTTP API ではありません。`ctx` コマンドに `--format json` を指定し、標準出力へ返された JSON を利用します。

## 用途

JSON API は、次のような外部ツールから `ctx` の情報を再利用するために使用します。

- 保存済みターゲット情報の取得
- 保存済み認証情報の取得
- 保存済みサービス情報の取得
- 現在の Workspace 状態の取得
- 利用可能な JSON 出力バージョンの確認

SQLite データベースを直接参照することもできますが、DB スキーマへの依存を避けたい場合は JSON API の利用を推奨します。

## 基本的な使い方

```bash
ctx <command> [arguments] --format json [--format-version <version>]
```

例:

```bash
ctx prompt --format json
ctx credential ls ssh --format json
ctx service ls --format json
ctx formats --format json
```

フォーマットバージョンを固定する場合:

```bash
ctx prompt --format json --format-version 1.0
```

## フォーマットバージョン

フォーマットバージョンは `MAJOR.MINOR` 形式です。

```text
1.0
1.1
2.0
```

指定方法:

```text
未指定  利用可能な全バージョンのうち最新
1       1.x のうち最新
1.1     1.1 を固定
```

例:

```bash
ctx prompt --format json
ctx prompt --format json --format-version 1
ctx prompt --format json --format-version 1.1
```

レスポンス内の `format_version` には、実際に使用された完全なバージョンが入ります。

```json
{
  "format_version": "1.1"
}
```

## 共通レスポンス

すべての JSON 出力は、同じ外枠を使用します。

### 成功時

```json
{
  "success": true,
  "format_version": "1.0",
  "data": {},
  "error": null
}
```

### 失敗時

```json
{
  "success": false,
  "format_version": "1.0",
  "data": null,
  "error": {
    "code": "NOT_FOUND.WORKSPACE",
    "message": "no active workspace",
    "details": {}
  }
}
```

### フィールド

| フィールド | 型 | 説明 |
|---|---|---|
| `success` | boolean | 処理が成功したか |
| `format_version` | string \| null | 実際に使用されたフォーマットバージョン |
| `data` | object \| array \| null | 成功時のデータ |
| `error` | object \| null | 失敗時のエラー情報 |

## エラー

### 構造

```json
{
  "code": "NOT_FOUND.WORKSPACE",
  "message": "no active workspace",
  "details": {}
}
```

- `code`: 外部ツールが条件分岐に使用する機械向けコード
- `message`: 英語の人間向けメッセージ
- `details`: 生のエラー情報や補助情報

外部ツールは `message` や `details` の内容に依存せず、`code` を使用してください。

### 親コード

現時点で共通仕様として定義されている親コード:

```text
INVALID_REQUEST
NOT_FOUND
INTERNAL_ERROR
```

子コードは親コードを接頭辞として表現します。

```text
INVALID_REQUEST.FORMAT_VERSION
NOT_FOUND.WORKSPACE
```

未知の子コードを受け取った場合は、最初の `.` より前を親コードとして扱えます。

```text
NOT_FOUND.WORKSPACE
→ NOT_FOUND
```

## 出力規則

`--format json` を指定した場合、標準出力には JSON のみが出力されます。

- JSON: 標準出力
- 警告・診断・デバッグ情報: 標準エラー出力

JSON を生成可能なエラーでは、想定外のエラーを含めて `success: false` の共通レスポンスを返します。

## 値が存在しない場合

選択されたフォーマットバージョンで定義されているフィールドは、値がなくても必ず返されます。

```text
単一値なし      null
一覧なし        []
追加情報なし    {}
```

例:

```json
{
  "username": "admin",
  "password": null
}
```

外部ツールは、定義済みフィールドが常に存在する前提で実装できます。

## 終了コード

```text
0  success
1  execution error
2  invalid arguments
```

詳細な判定には終了コードではなく `error.code` を使用してください。

## 利用可能な JSON 出力

初期対応対象:

```text
formats
prompt
credential
service
```

## `formats`

利用可能な JSON 出力名と、それぞれが対応しているフォーマットバージョンを返します。

```bash
ctx formats --format json --format-version 1.0
```

`--format json` を付けない場合は、同じ情報を表形式で表示します。

```text
OUTPUT       VERSIONS
credential   1.0
formats      1.0
prompt       1.0
service      1.0
```

Add-on はこの出力を使用して、必要な JSON 出力とバージョンが利用可能か確認できます。

## `prompt`

現在の Workspace、Primary Target、Local IP などの実行コンテキストを返します。

```bash
ctx prompt --format json --format-version 1.0
```

例:

```json
{
  "success": true,
  "format_version": "1.0",
  "data": {
    "active": true,
    "workspace_id": "fa874e0a-c4d5-41fa-b6ba-63687d58a737",
    "workspace_name": "aaa",
    "workspace_path": "/workspace/cases/aaa",
    "local_ip": "172.18.0.2",
    "local_interface": "eth0",
    "target_name": "default",
    "target_ip": "1.2.3.4"
  },
  "error": null
}
```

## `credential`

保存済み認証情報を返します。

```bash
ctx credential ls --format json --format-version 1.0
ctx credential ls ssh --format json --format-version 1.0
```

`scope` を指定した場合は、その scope に一致する認証情報だけを返します。

並び順:

```text
scope ASC, username ASC, id ASC
```

パスワードは平文で返されます。外部ツールは、取得したパスワードをログ、標準エラー出力、一時ファイル、プロセス一覧へ不用意に露出させないでください。

## `service`

保存済みサービス情報を返します。

```bash
ctx service ls --format json --format-version 1.0
```

並び順:

```text
protocol ASC, port ASC, id ASC
```

## 互換性

フォーマットバージョンは JSON の構造だけを管理します。

### MAJOR

基礎構造を壊す変更で更新されます。

例:

- 基礎フィールドの削除
- フィールド名の変更
- 型変更
- 意味変更
- null 許容変更
- ネスト構造変更
- 配列とオブジェクトの変更

### MINOR

同じ MAJOR の基礎構造を維持した拡張で更新されます。

同じ MAJOR 内で過去の MINOR に追加された任意フィールドは、後続 MINOR で削除または再構成される場合があります。

### 同一バージョン

同じ `MAJOR.MINOR` では JSON 構造を固定します。

次の変更ではフォーマットバージョンを更新しません。

- 値の取得バグ修正
- 内部実装の変更
- 値の修正
- 仕様違反の並び順修正
- JSON エスケープ不具合の修正

## バージョンの維持

公開済みフォーマットバージョンは原則として維持します。

ただし、互換実装の維持が過度な負担になった場合は、下位互換を打ち切ることがあります。恒久的な維持は保証されません。

打ち切る場合は、対象 JSON 出力と対象バージョンを明示します。
