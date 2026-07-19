# データベース設計

## 概要

`ctx` は内部状態の管理に SQLite を使用します。

データベースは `ctx` の内部実装です。外部ツールや Add-on は、可能な限りデータベーススキーマへ依存せず、JSON API を利用してください。

## 目的

データベース設計では、次の要件を満たすことを目的とします。

- アップデート後もユーザーデータを保持する
- スキーマを自動で更新できる
- シンプルで保守しやすい構造を維持する
- 将来のスキーマ変更へ対応できる

## マイグレーション方針

正式リリースされた `ctx` のすべてのバージョンは、それ以前の正式リリース版で作成されたデータベースを最新版まで更新できなければなりません。

スキーママイグレーションは、互換性保証の一部として扱います。

なお、正式リリースされていない開発中のスキーマについては、互換性を保証しません。

## スキーマバージョン

データベーススキーマのバージョンは、アプリケーションのバージョンとは独立して管理します。

データベースを開く際に現在のスキーマバージョンを確認し、必要なマイグレーションを適用した後で通常処理を開始します。

現在のソースツリーのマイグレーションバージョンは`3`です。ctx本体のバージョンからスキーマバージョンを推測せず、データベースを直接確認します。

```sh
DB="${CTX_HOME:-$HOME/.ctx}/db.sqlite"
sqlite3 -readonly "$DB" 'SELECT version, dirty FROM schema_migrations;'
```

`dirty = 0`は、記録されたマイグレーションが完了していることを示します。`schema_migrations`を手動で変更してはいけません。

## データベースの場所

ctxは全workspaceで1つの共有データベースを使用します。

```text
${CTX_HOME:-$HOME/.ctx}/db.sqlite
```

`${CTX_HOME:-$HOME/.ctx}/workspaces/<uuid>/`配下はツールの状態やスキャン結果であり、workspaceごとのctxデータベースではありません。`ctx status`は手動確認用に解決済みのデータベースパスを表示しますが、人間向け出力は機械連携仕様の対象ではありません。

`CTX_HOME`を設定するとctxのdata root全体が変わります。継続的な互換性が必要なカスタムコマンドは、この内部パスを組み立てず、文書化されたJSON APIと登録コマンドを利用してください。

## 読み取り専用での確認

SQLite CLIの読み取り専用モードを使用します。確認前に一度ctxを起動し、保留中のマイグレーションを外部スクリプトではなくctx自身に適用させます。

```sh
DB="${CTX_HOME:-$HOME/.ctx}/db.sqlite"
ctx status >/dev/null
sqlite3 -readonly "$DB"
```

対話操作で利用できる確認コマンド:

```sql
.tables
.schema targets
SELECT version, dirty FROM schema_migrations;
PRAGMA integrity_check;
```

読み取り専用queryの例:

```sql
SELECT w.name AS workspace, t.name AS target, t.ip, t.is_primary
FROM workspaces AS w
JOIN targets AS t ON t.workspace_id = w.id
ORDER BY w.name, t.id;

SELECT w.name AS workspace, t.ip, s.port, s.protocol, s.service_name, s.product, s.version
FROM services AS s
JOIN targets AS t ON t.id = s.target_id
JOIN workspaces AS w ON w.id = t.workspace_id
ORDER BY w.name, t.id, s.port, s.protocol;
```

table名、column、constraint、relationは内部実装であり、ctx更新時に変更される可能性があります。あるschema versionで動作するqueryが、別versionでも動作することは保証しません。

## 直接利用前のバックアップ

稼働中のdatabase fileを`cp`せず、SQLiteの整合したbackupを作成します。

```sh
DB="${CTX_HOME:-$HOME/.ctx}/db.sqlite"
BACKUP="${DB}.backup-$(date +%Y%m%d-%H%M%S)"
sqlite3 "$DB" ".backup '$BACKUP'"
sqlite3 -readonly "$BACKUP" 'PRAGMA integrity_check;'
```

backupにはcredentialの平文password、command output、Cookie、token、target情報、noteが含まれる可能性があります。機密性のある調査データとして保存・転送してください。

SQLを試す場合は、backupまたは別の使い捨てcopyを開きます。変更した実験用databaseを通常のctxコマンドから利用してはいけません。

## 直接書き込みの警告

直接書き込みは非対応です。ctxの入力検証、target・workspace解決、関連record更新、重複排除規則、command log lifecycle検証、migrationを迂回します。foreign keyやunique constraintだけでは、これらの動作を再現できません。

稼働中のデータベースに対してinsert、update、delete、create、drop、alterを実行しないでください。カスタムコマンドから結果を保存する場合は、既存のctx登録コマンドを使用します。具体的な処理を既存の外部連携仕様で表現できない場合だけ、新しい機械入力操作を検討します。

特に`credentials.password`、`command_logs.stdout`、`command_logs.stderr`は機密情報を含みます。診断出力へ表示したり、query結果をshell historyや通常のlog fileへ保存したりしないでください。

## 実装方針

`ctx` のデータベース基盤は、次の方針で実装します。

- マイグレーションライブラリは `golang-migrate` を採用する。
- 最新スキーマスナップショットとマイグレーションファイルは `go:embed` を使用してバイナリへ埋め込む。
- スキーマバージョンは `golang-migrate` が管理する `schema_migrations` テーブルを使用する。
- データベースを開く際に、必要なマイグレーションを自動で適用する。
- 各マイグレーションはトランザクション内で実行し、失敗した場合はロールバックして通常処理を開始しない。
- 新規データベースは `internal/ctx/schema.sql` から直接作成し、過去のマイグレーションは実行しない。
- 既存データベースは不足しているマイグレーションを順番に適用する。
- `schema_migrations` が存在しないデータベースは、既知の正式リリース版スキーマと一致することを検証できた場合にのみ legacy database として扱う。
- 通常運用ではダウングレードは行わない。`.down.sql` は開発・検証用途として保持する。
- 正式リリース版ごとにデータベースフィクスチャを保持する。
- フィクスチャは現在のソースコードから再生成せず、実際の正式リリース版 ctx が作成したデータベースを使用する。
- CIではすべての正式リリース版フィクスチャを最新版まで更新できることを確認する。

## 最新スキーマスナップショット

`internal/ctx/schema.sql` は、新規データベース作成専用の最新完成形スキーマです。

新規データベースを作成する場合、`ctx` は `schema.sql` を直接適用し、その後 `schema_migrations` に最新マイグレーション番号を記録します。

`schema.sql` はマイグレーション履歴ではありません。現在のソースツリーにおける最終スキーマのスナップショットです。

## マイグレーションファイル

マイグレーションファイルは連番順に適用します。

標準形式:

```text
<連番>_<ctxバージョン>.up.sql
<連番>_<ctxバージョン>.down.sql
```

例:

```text
000001_1.0.0.up.sql
000001_1.0.0.down.sql

000002_1.1.0.up.sql
000002_1.1.0.down.sql
```

必要に応じて、変更内容をファイル名へ追加できます。

```text
<連番>_<ctxバージョン>_<変更内容>.up.sql
<連番>_<ctxバージョン>_<変更内容>.down.sql
```

例:

```text
000003_1.2.0_rework_workspaces.up.sql
000003_1.2.0_rework_workspaces.down.sql
```

## マイグレーションルール

- 原則として、1つの `ctx` リリースにつき1つのスキーママイグレーションとします。
- 同一リリース内で複数のスキーマ変更が必要な場合は、可能な限り1つのマイグレーションへまとめます。
- 一度正式リリースしたマイグレーションファイルは変更・削除しません。
- 正式リリース後の修正は、新しいマイグレーションとして追加します。

## 新規データベース

新しくデータベースを作成する場合は、最新のスキーマを直接適用します。

過去のすべてのマイグレーションを順番に実行してデータベースを構築してはいけません。

最新スキーマスナップショットを適用した後、データベースは最新マイグレーション番号として記録されなければなりません。

## 既存データベース

既存のデータベースは、現在のスキーマバージョンから最新版まで、すべてのマイグレーションを順番に適用して更新します。

途中のマイグレーションを省略することはできません。

既存データベースに `schema_migrations` が存在しない場合でも、推測でバージョンを設定してはいけません。まず、既知の正式リリース版スキーマと一致することを検証する必要があります。

`ctx v1.0.0` の baseline では、最低限次を確認します。

- 必須テーブルがすべて存在する
- 必須カラムが存在する
- カラム型と主要な制約が一致する
- 必要なインデックスが存在する
- `PRAGMA integrity_check` が `ok` を返す

検証に成功した場合のみ、`schema_migrations` に version `1` を記録して baseline とします。

検証に失敗した場合は、明示的なエラーで起動を中止し、通常処理を開始してはいけません。

## テストフィクスチャ

正式リリース版ごとに、実際のリリース版 ctx が作成したデータベースをテストフィクスチャとして保持します。

フィクスチャはテスト専用の固定データであり、一度追加した後は内容を変更しません。

テストではフィクスチャを直接変更せず、一時コピーを作成してマイグレーションを実行します。

各フィクスチャについて、生成元となるリリースタグまたはコミットを記録します。

## 互換性保証

`ctx` は、正式リリース版で作成されたデータベースが、将来の正式リリース版へ更新できることを保証します。

この互換性の維持は、`ctx` の重要な設計方針の一つです。
