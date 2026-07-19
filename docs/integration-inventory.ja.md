# ctx連携実装一覧

この文書は、現在の各バイナリがctxとどのように連携しているかを記録します。外部向けAPI仕様ではなく、実装の棚卸しです。目標とする所有境界と利用者向けの選択方法は[ctx連携ガイド](integration.ja.md)で定義します。

## 連携経路

現在のリポジトリには、主に2つの連携経路があります。

1. `xssh`、`xftp`、`xsmb`、`xscp`は`ctx`プロセスを実行し、バージョン付きJSONを利用する
2. `xgobuster`、`xhydra`、`xffuf`、`xwebshell`は`internal/ctx`をimportし、Go関数を直接呼び出す

`xgobuster`は両方を使っています。promptとserviceはctxプロセスのJSONから読み取り、それ以外の処理には`internal/ctx`を使用します。

SQLiteをSQLで直接開く`x*`コマンドはありません。ただし`internal/ctx`経由の呼び出しはDBを読み書きできるため、外部連携仕様ではなく内部実装への直接依存です。

## 能力別一覧

| 能力 | 公開JSONをプロセス経由で使うコマンド | `internal/ctx`を直接使うコマンド | 現在の不足 |
|---|---|---|---|
| 現在のworkspaceとtarget | `xssh`、`xftp`、`xsmb`、`xscp`、`xgobuster`が`prompt`を使用 | `xhydra`、`xffuf`、`xwebshell`が`LoadPromptData`を使用し、探索ツールはworkspaceとprimary targetも取得 | 直接利用側はGo内部実装へ依存 |
| Host | なし | `xgobuster`、`xffuf`が`ListHosts`を使用 | hostのJSON出力がない |
| Service | `xssh`、`xftp`、`xsmb`、`xscp`、`xgobuster`が`service`を使用 | `xhydra`、`xffuf`が`ListServices`を使用 | service読込・選択が二系統ある |
| Credential参照 | `xssh`、`xftp`、`xsmb`、`xscp`が`credential`を使用 | なし | JSONレスポンスとfilter処理が4重実装 |
| Credential登録 | なし | `xhydra`が`SetCredential`を使用 | 内製コマンドは既存CLI登録を利用していない |
| Host登録 | なし | `xgobuster`、`xffuf`が`AddHost`を使用 | 内製コマンドは既存CLI登録を利用していない |
| Service登録 | なし | `ctx scan`が`UpsertService`を使用し、別の`x*`書込側はない | 公開service登録操作がない |
| Note | なし | なし | カスタムコマンドは通常の`ctx note`だけ利用可能 |
| Command log | `xssh`、`xftp`、`xsmb`、`xscp`が`log start/finish`を使用 | `xgobuster`、`xhydra`、`xffuf`が`StartCommandLog`と`FinishCommandLog`を使用 | JSON loggerが重複し、直接利用側は迂回している |
| Web探索結果 | なし | `xgobuster`が保存・一覧取得 | 公開Web探索操作がない |
| Web wordlist実行履歴 | なし | `xgobuster`、`xffuf`が開始・終了・一覧取得 | 公開実行履歴操作がない |
| 設定 | なし | `xgobuster`、`xhydra`、`xffuf`が`LoadConfig`を使用 | 機械可読なconfig APIがなく、Go structへ依存 |
| Wordlist探索 | なし | `xgobuster`、`xhydra`、`xffuf`がctxのwordlist helperを使用 | 選択方針とctx永続化が1つの内部packageで結合 |
| 機能・format確認 | なし | なし | endpoint利用前に`ctx formats`を呼ぶアドオンがない |

公開操作が存在しないという結果だけで、API追加が決まるわけではありません。既存コマンドでは安全に実現できない具体的な連携が発生した場合だけ追加を検討します。

## コマンド別一覧

### `xssh`

- JSON参照: `prompt`、`credential ls ssh`、`service ls`
- JSON書込: `log start`、`log finish`
- JSONクライアント: 共通`internal/ctxapi`
- 人間向けctxコマンド: なし
- `internal/ctx`: なし
- ctx設定: なし
- private state: 最後に選んだcredential IDを`${XDG_STATE_HOME:-$HOME/.local/state}/xssh/state`へ保存
- 共有結果の書込: command logのみ

### `xftp`

- JSON参照: `prompt`、`credential ls ftp`、`service ls`
- JSON書込: `log start`、`log finish`
- JSONクライアント: 共通`internal/ctxapi`
- 人間向けctxコマンド: なし
- `internal/ctx`: なし
- ctx設定: なし
- private state: 最後に選んだcredential IDを`${XDG_STATE_HOME:-$HOME/.local/state}/xftp/state`へ保存
- 共有結果の書込: command logのみ

### `xsmb`

- JSON参照: `prompt`、`credential ls smb`、`service ls`
- JSON書込: `log start`、`log finish`
- JSONクライアント: 共通`internal/ctxapi`
- 人間向けctxコマンド: なし
- `internal/ctx`: なし
- ctx設定: なし
- private state: 最後に選んだcredential IDを`${XDG_STATE_HOME:-$HOME/.local/state}/xsmb/state`へ保存
- 共有結果の書込: command logのみ

### `xscp`

- JSON参照: `prompt`、`credential ls ssh`、`service ls`
- JSON書込: `log start`、`log finish`
- JSONクライアント: 共通`internal/ctxapi`
- 人間向けctxコマンド: なし
- `internal/ctx`: なし
- ctx設定: なし
- private state: xsshと同じ`${XDG_STATE_HOME:-$HOME/.local/state}/xssh/state`を使用
- 共有結果の書込: command logのみ

### `xgobuster`

- JSON参照: `prompt`、`service ls`
- JSONクライアント: 共通`internal/ctxapi`
- JSON書込: なし
- 人間向けctxコマンド: なし
- `internal/ctx`参照: workspace、primary target、host、設定、wordlist定義、Web探索結果、Web wordlist実行履歴
- `internal/ctx`書込: host、command log、Web探索結果、Web wordlist実行履歴
- ctx設定: `web.directory.max-requests`、`web.file.max-requests`、`dns.max-queries`、`web.tls.verify`
- workspace内private state: Web・DNS検索済みword、拡張子別検索範囲、有効なWeb探索strategy
- 一時状態: 間引いたwordlist
- 共有結果の書込: host、Web探索結果、command log、wordlist実行履歴

### `xhydra`

- JSONプロセス呼び出し: なし
- 人間向けctxコマンド: なし
- `internal/ctx`参照: prompt data、workspace、primary target、service、設定、password・username wordlist定義
- `internal/ctx`書込: credential、command log
- ctx設定: `password.max-requests`
- workspace内private state: target、service、endpoint、固定入力ごとの検索済みpassword・username
- 一時状態: 間引いたpasswordまたはusername list
- 共有結果の書込: 成功したcredential、command log

### `xffuf`

- JSONプロセス呼び出し: なし
- 人間向けctxコマンド: なし
- `internal/ctx`参照: prompt data、workspace、primary target、host、service、設定、wordlist定義
- `internal/ctx`書込: host、command log、Web wordlist実行履歴
- ctx設定: `web.vhost.max-requests`、`web.vhost.calibration-samples`、`web.vhost.calibration-confidence`、`web.tls.verify`
- workspace内private state: target、URL、domainごとの検索済みvhost word
- 一時状態: calibration wordlist、間引いたwordlist、ffuf JSON結果ファイル
- 共有結果の書込: 確定したhost、command log、Web wordlist実行履歴。trial modeでは書き込まない

### `xwebshell`

- JSON参照: `prompt`
- JSONクライアント: 共通`internal/ctxapi`
- 人間向けctxコマンド: なし
- `internal/ctx`: なし
- ctx設定: なし
- private state: なし
- 外部ファイル: system webshell templateを読み、設定済みファイルをcurrent directoryへ出力
- 共有結果の書込: なし

### `req`

`req`は意図的に独立しています。ctxの実行、`internal/ctx`のimport、ctx設定の参照、ctx workspace stateの利用は行いません。

## 現在の重複

接続・転送コマンドのJSON通信は`internal/ctxapi`へ共通化済みです。次のツール固有処理は意図的に分離したまま維持します。

- prompt、credential、serviceのdata struct
- protocol固有のservice filterとcommand組み立て
- 子プロセスerrorの変換
- 最後に選んだcredential state。ただしxscpは意図的にxsshのstateを共有

serviceのfilterと対話選択も複数ツールにありますが、protocol固有の判定規則は異なります。

`internal/ctxapi`は、バージョン管理された既存のJSON APIを利用する共通クライアントです。JSON出力用引数の付与、標準入力からのJSON送信、共通レスポンスとフォーマットバージョンの検証、APIエラーの保持、不正なJSON・データ欠落・プロセス失敗の判別を担当します。`xssh`はprompt、credential、service、log操作にこのクライアントを使用します。

プロセス経由のctx呼び出しは`internal/ctxexec`を使用し、固定絶対パス`/usr/local/bin/ctx`をシェルなしで実行します。別の配布物ではビルド時にパスを変更でき、テストでは差し替えられます。

## 現在の不整合と不足

- 最初のJSON request前に`ctx formats`でendpoint対応状況を確認するアドオンがない
- 接続系アドオンは公開JSON、探索・認証ツールは`internal/ctx`を使い、公式コマンド内に異なる互換境界がある
- host、config、Web探索結果、Web wordlist履歴には公開JSON操作がなく、そのままでは直接依存を移行できない
- 内製の書込側は既存の人間向け登録コマンドではなく`AddHost`や`SetCredential`を直接呼ぶ。既存コマンドには、すべての用途を満たす安全な機械処理向け入力仕様がまだない
- 人間向けctx tableを解析する`x*`コマンドは存在せず、この点は望ましい状態になっている
- SQLを直接実行する`x*`コマンドは存在せず、`internal/ctx`外へDB schema knowledgeは重複していない

これらは後続TODOの判断材料です。この結果だけで、すべての公式コマンドをプロセスAPIへ移行するとは決めません。

## 移行判断

| コマンド | 判断 | 理由 |
|---|---|---|
| `xftp` | prompt、credential、service、logを`internal/ctxapi`へ移行 | ctx操作がすべて外部向けのバージョン管理されたJSON APIで表現済みであり、挙動を変えずに重複した通信処理を削除できる |
| `xsmb` | prompt、credential、service、logを`internal/ctxapi`へ移行 | xsshと同じく公開APIだけで完結し、SMB固有の選択処理はコマンド側へ残せる |
| `xscp` | prompt、credential、service、logを`internal/ctxapi`へ移行 | xsshと同じく公開APIだけで完結し、xsshのcredential state共有はツール固有処理として維持できる |
| `xgobuster` | promptとservice参照は`internal/ctxapi`を使い、その他の`internal/ctx`操作は維持 | この2つは外部向けJSON APIで表現できる一方、host、設定、ログ、探索結果、wordlist履歴には同等の外部連携仕様がない |
| `xhydra` | 当面`internal/ctx`を維持 | 設定、wordlist方針、scope付きcache、ログ、credential書込が一体であり、一部だけプロセス移行しても内部依存を削除できず境界が増える |
| `xffuf` | 当面`internal/ctx`を維持 | 設定、service、host、calibration state、ログ、host書込、wordlist履歴が一体であり、必要な公開操作が揃っていない |
| `xwebshell` | prompt参照に`internal/ctxapi`を使用 | callback IP選択に公開prompt dataだけを使用し、`internal/ctx`依存はない |

維持すると判断した直接依存は、内製コマンドの明示的な実装判断であり、利用者コマンド向けの公開APIではありません。単一の参照だけを移すのではなく、実際の処理全体を置き換えられる公開操作が具体化した時点で再評価します。
