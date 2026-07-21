# xwebshell オンラインヘルプ

[English](./xwebshell.md)

`xwebshell` はKaliの `/usr/share/webshells` にあるWeb shell templateをcatalog化し、一覧、内容確認、設定済みcopyのexportを行うCLIです。許可された演習環境だけで使用してください。

## 構文

```text
xwebshell [ls]
xwebshell show <ID>
xwebshell export <ID>
```

```sh
xwebshell
xwebshell show 3
xwebshell export 3
```

引数なしは `xwebshell ls` と同じです。

## `ls`

内蔵catalogと実際のfilesystemを比較し、ID、status、category、nameを表示します。

```text
[+] catalog登録済みで利用可能
[!] catalog登録済みだがfile/packageが見つからない
[?] filesystem上で検出した未登録file
```

末尾にはAvailable、New、Missingの件数を表示します。IDはその時点の一覧に対応するため、package内容が変わると変動する可能性があります。

`/usr/share/webshells` を再帰的に確認し、catalogでdirectory単位に管理するentryは配下fileをまとめて扱います。利用可能なSecLists由来のWeb shellも検出対象に含めます。

## `show <ID>`

entryの説明、絶対path、含まれるfileと内容を表示します。group entryでは配下fileを列挙します。

secretやcallback先を含む設定済みtemplateを表示するとterminal scrollbackへ残るため注意してください。`show` はsourceを変更しません。

## `export <ID>`

利用可能status `[+]` のtemplateをcurrent directoryへcopyします。templateに設定項目がある場合は、callback host/portなどを対話入力してcopy側へ反映します。

```sh
cd /workspace/thm/room
xwebshell export 3
```

sourceの `/usr/share/webshells` は変更しません。出力pathが既に存在する場合は上書きせず失敗します。group entryはdirectory構造を保ってexportします。

`[!]` はsourceがないためexportできません。`[?]` はcatalog metadataや設定規則を持たないため、一覧で確認後に元fileを直接扱ってください。

## Packageと検出範囲

基本sourceはKaliの `webshells` packageです。packageがinstallされておらず `/usr/share/webshells` が存在しない場合はerrorになります。環境によってSecListsなど任意packageの有無が違っても、存在する範囲を動的に一覧化します。

## 安全性

- Web shellはremote command executionを目的とするcodeです。
- export後のfileを誤って公開server、Git repository、artifactへ置かないでください。
- callback IP/portやauthentication情報を確認してから使用してください。
- source packageは変更せず、current directoryのcopyだけを編集します。
- uploadと実行は明示的許可を得た演習対象に限定してください。

## Shell completion

`ctx init-shell` で導入されるcompletionは、`show` / `export` のID、category/name、descriptionを候補として表示します。内部の `__complete` commandはcompletion実装用で、通常は直接使用しません。

## よくある問題

- `webshells package not found`: Kali serviceへ `webshells` packageをinstallします。
- `[!]` が多い: 任意provider/packageがinstallされていないか、Kali package構成がcatalogと異なります。
- `[?]` が出る: package側に追加された未登録fileです。`show` 対象としてのmetadataがないためpathを直接確認します。
- export先が既にある: 既存fileを保護する仕様です。別directoryへ移動するか、内容を確認して自分で整理します。
