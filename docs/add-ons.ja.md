ctxを利用したツールを書く人向け

xssh
xhydra
xtelnet
xftp
独自ツール

# Add-ons

`ctx` は単体のCLIツールであると同時に、他のCLIツールが利用できる共通基盤として設計されています。

Add-on は `ctx` に保存された情報を利用して作業を効率化する独立したツールです。

## 目的

- `ctx` に保存された情報を再利用する
- 同じ情報を何度も入力しない
- 各ツールは 1 つの役割に集中する

## 設計方針

- Add-on は `ctx` とは別パッケージとして配布する
- Add-on は `ctx` がインストールされていることを前提とする
- Add-on は `ctx` の内部データベースを直接参照しない
- `ctx` が提供する JSON API を利用する
- Add-on は必要な情報のみを `ctx` から取得する

## JSON API

Add-on は機械可読な JSON 出力を利用して `ctx` と連携します。

例:

```bash
ctx prompt --format json --format-version 1
ctx credential ls ssh --format json --format-version 1
ctx service ls --format json --format-version 1
```

JSON API の詳細は `json-api.md` を参照してください。

## Add-on の例

- `xssh` - 保存済み SSH 認証情報を利用して接続
- `xhydra` - 保存済み認証情報を利用して Hydra を実行
- `xtelnet` - 保存済み Telnet 認証情報を利用して接続
- `xftp` - 保存済み FTP 認証情報を利用して接続

## 開発ガイドライン

- 1 つの Add-on は 1 つの責務を持つ
- 認証情報やターゲット情報は `ctx` に保存し、自前で管理しない
- 将来の互換性のため、JSON API のバージョンを明示して利用する
- `ctx` の公開 API のみを利用し、内部実装には依存しない
