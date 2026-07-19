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
NOT_FOUND.TARGET
NOT_FOUND.LOG
```

現在のエラーコード:

| コード | 終了コード | 意味 |
|---|---:|---|
| `INVALID_REQUEST` | 2 | コマンド引数またはJSON request bodyが不正 |
| `INVALID_REQUEST.FORMAT_VERSION` | 2 | 要求したフォーマットバージョンが不正または未対応 |
| `NOT_FOUND.WORKSPACE` | 1 | 有効なWorkspaceが存在しない |
| `NOT_FOUND.TARGET` | 1 | Primary Targetまたは指定したTargetが存在しない |
| `NOT_FOUND.LOG` | 1 | 指定したコマンドログが存在しない |
| `INTERNAL_ERROR` | 1 | 予期しないDB、ファイルシステム、実装上の障害 |

利用者が修正できる入力エラーとリソース不存在は、`INTERNAL_ERROR`として返しません。

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

`--format json`を指定した後は、コマンド引数が不正な場合も共通レスポンスを使用します。この場合、usage情報をJSONレスポンスの外へ混在させません。

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
log
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
log          1.0
prompt       1.0
service      1.0
```

Add-on はこの出力を使用して、必要な JSON 出力とバージョンが利用可能か確認できます。

`data` オブジェクトの構造:

```json
{
  "formats": {
    "credential": ["1.0"],
    "formats": ["1.0"],
    "log": ["1.0"],
    "prompt": ["1.0"],
    "service": ["1.0"]
  }
}
```

| フィールド | 型 | 説明 |
|---|---|---|
| `formats` | object | JSON 出力名を、その出力が対応するフォーマットバージョンへ対応付けるオブジェクト |
| `formats.<name>` | string の配列 | 対応バージョン。バージョンの昇順 |

`formats` オブジェクト内のキー順に意味はありません。未知の出力名が存在することを前提にしないでください。

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

`data` オブジェクトのフィールド:

| フィールド | 型 | 説明 |
|---|---|---|
| `active` | boolean | カレントディレクトリが有効な Workspace に属しているか |
| `workspace_id` | string \| null | Workspace ID |
| `workspace_name` | string \| null | Workspace 名 |
| `workspace_path` | string \| null | Workspace の絶対パス |
| `local_ip` | string \| null | ctx が選択したローカルの callback IP |
| `local_interface` | string \| null | `local_ip` に対応するインターフェース |
| `target_name` | string \| null | Primary Target 名 |
| `target_ip` | string \| null | Primary Target の IP アドレス |

`active` が `false` の場合、他のフィールドはすべて `null` です。有効な Workspace 内でも値を取得できないフィールドは `null` になります。

## `credential`

保存済み認証情報を返します。

```bash
ctx credential ls --format json --format-version 1.0
ctx credential ls ssh --format json --format-version 1.0
```

`scope` を指定した場合は、その scope に一致する認証情報だけを返します。

`data` オブジェクトの例:

```json
{
  "credentials": [
    {"id": 1, "scope": "ssh", "username": "root", "password": "toor"},
    {"id": 2, "scope": "ssh", "username": "testuser", "password": null}
  ]
}
```

| フィールド | 型 | 説明 |
|---|---|---|
| `credentials` | object の配列 | 現在の Workspace に保存された認証情報。一致する情報がなければ `[]` |
| `credentials[].id` | integer | 認証情報レコードの ID |
| `credentials[].scope` | string | 認証情報の scope |
| `credentials[].username` | string | ユーザー名 |
| `credentials[].password` | string \| null | パスワード。保存されていなければ `null` |

並び順:

```text
scope ASC, username ASC, id ASC
```

パスワードは平文で返されます。外部ツールは、取得したパスワードをログ、標準エラー出力、一時ファイル、プロセス一覧へ不用意に露出させないでください。

## `log`

Add-on は ctx の DB を直接参照せずに、コマンドログを開始・完了できます。リクエストは標準入力から JSON で渡し、レスポンスは標準出力へ JSON で返します。

どちらの操作にも有効なWorkspaceが必要です。以下に記載していないリクエストフィールドは、バージョン管理された仕様には含まれません。

### ライフサイクル

処理を開始する直前に`start`を呼び出します。ctxは状態が`running`のログを作成してIDを返します。そのIDをメモリ内で保持し、処理が次のいずれかの終端状態になった後に`finish`を1回呼び出します。

```text
running -> success
running -> failed
running -> interrupted
```

処理が正常に完了した場合だけ`success`、通常の失敗は`failed`、完了前にキャンセルまたは中断された場合は`interrupted`を使用します。処理開始後に`finish`を呼ばずAdd-onが終了すると、ログは`running`のまま残ります。キャンセル時も可能な限り`interrupted`で完了してください。

ログを開始する場合:

```bash
printf '%s\n' '{"command":"xssh","expanded_command":"ssh -p 22 testuser@172.18.0.5","started_at":"2026-07-13T00:00:00Z"}' | \
  ctx log start --format json --format-version 1.0
```

レスポンスには新しいログ ID が含まれます:

```json
{
  "success": true,
  "format_version": "1.0",
  "data": {"id": 1},
  "error": null
}
```

開始リクエストのフィールド:

| フィールド | 型 | 必須 | 説明 |
|---|---|---|---|
| `command` | string | 必須 | 利用者向けのコマンド名。空白のみは不可 |
| `expanded_command` | string | 任意 | 展開後のコマンド。省略または空白の場合は `command` を使用 |
| `started_at` | string | 任意 | 開始時刻。省略または空白の場合は現在の UTC 時刻を RFC 3339 形式で設定 |

成功時の `data.id` は、新しく作成されたコマンドログ ID を表す integer です。

呼び出し側が指定した時刻文字列は、そのまま保存します。ctxと他のツールが一貫して並び替え・表示できるよう、RFC 3339形式のUTC時刻を使用してください。

ログを完了する場合は、結果を JSON で渡します。command や出力へパスワード、`sshpass` の引数を含めないでください。

```bash
printf '%s\n' '{"status":"success","exit_code":0,"stdout":"connected\n","stderr":"","ended_at":"2026-07-13T00:05:00Z"}' | \
  ctx log finish 1 --format json --format-version 1.0
```

完了リクエストのフィールド:

| フィールド | 型 | 必須 | 説明 |
|---|---|---|---|
| `status` | string | 必須 | `success`、`failed`、`interrupted` のいずれか |
| `exit_code` | integer \| null | 任意 | プロセスの終了コード。省略または `null` の場合は `0` |
| `stdout` | string | 任意 | 取得した標準出力。省略時は空文字列 |
| `stderr` | string | 任意 | 取得した標準エラー出力。省略時は空文字列 |
| `ended_at` | string | 任意 | 終了時刻。省略または空白の場合は現在の UTC 時刻を RFC 3339 形式で設定 |

`<id>` 引数には、既存のコマンドログを示す正の整数を指定します。完了成功時も開始時と同じ形式で、`data.id` に対象のログ ID を返します。

### 秘密情報

command、expanded command、stdout、stderrはWorkspace DBへ永続化され、`ctx log`で表示できます。password、token、cookie、認証headerなどの秘密情報を取り除いてからrequestを送ってください。特に、`sshpass`などpasswordを含むコマンド引数を記録しないでください。

## `service`

保存済みサービス情報を返します。

```bash
ctx service ls --format json --format-version 1.0
ctx service ls --target web --format json --format-version 1.0
```

`--target` を省略すると Primary Target のサービスを返します。`--target <name>` を指定すると、その名前の Target のサービスを返します。

`data` オブジェクトの例:

```json
{
  "services": [
    {
      "id": 1,
      "port": 22,
      "protocol": "tcp",
      "state": "open",
      "reason": null,
      "service_name": "ssh",
      "product": "OpenSSH",
      "version": null,
      "extrainfo": null,
      "tunnel": null,
      "cpe": null,
      "last_seen": "2026-07-13T00:00:00Z"
    }
  ]
}
```

| フィールド | 型 | 説明 |
|---|---|---|
| `services` | object の配列 | 選択した Target に保存されたサービス。存在しなければ `[]` |
| `services[].id` | integer | サービスレコードの ID |
| `services[].port` | integer | ポート番号 |
| `services[].protocol` | string | トランスポートプロトコル |
| `services[].state` | string \| null | 検出したポート状態 |
| `services[].reason` | string \| null | 検出理由 |
| `services[].service_name` | string \| null | 検出したサービス名 |
| `services[].product` | string \| null | 検出した製品名 |
| `services[].version` | string \| null | 検出した製品バージョン |
| `services[].extrainfo` | string \| null | サービスの追加情報 |
| `services[].tunnel` | string \| null | `ssl` などのトンネル種別 |
| `services[].cpe` | string \| null | 検出した CPE 値 |
| `services[].last_seen` | string \| null | 最後にサービスを観測した時刻 |

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
