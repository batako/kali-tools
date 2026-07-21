# xsteg オンラインヘルプ

[English](./xsteg.md)

`xsteg` はlocal fileに埋め込まれたdataの候補を複数backendで調査し、安全な上限内で抽出するCLIです。`scan` は検出だけ、`extract` はscan結果に基づく抽出だけを担当します。

## 構文

```text
xsteg [ls [path]]
xsteg scan <path>
xsteg extract <path> [--auto | --manual] [-w <file>] [--no-crack]
xsteg show <ID> [path]
xsteg doctor
```

```sh
xsteg scan oneforall.jpg
xsteg extract oneforall.jpg
xsteg ls
xsteg show 1
```

引数なしはcurrent directoryに対する `xsteg ls` です。

## 推奨workflow

```text
xsteg scan file
  -> reportと候補を確認
xsteg show ID
  -> Findings / Tools / Warningsを確認
xsteg extract file
  -> scanで検出した候補だけを抽出
```

`extract` は、同じsource pathかつ現在のSHA-256と一致するcompleted scanを必要とします。scanせずに実行した場合は失敗し、空の `.xsteg` directoryを作りません。scan後にsourceを変更した場合も再scanが必要です。

## `scan`

fileまたはdirectoryを解析し、payloadを取り出さずreportを作ります。file typeに応じて利用可能なbackendを選びます。

- `file`: MIME/type判定
- `exiftool`: metadata確認
- `binwalk -B`: 埋め込みsignatureのoffset確認
- `strings`: printable string確認
- `steghide info`: JPEGなどのStegHide metadata候補
- `stegseek --seed`: password保護されたStegHide payload候補の高速判定
- `zsteg -a`: PNG/BMPのLSB候補

backendが未installの場合はskipまたはwarningとしてreportへ記録します。scanはpassword wordlistによる解析やpayload抽出を行いません。

### Binwalk結果の読み方

source自身のsignatureだけではembedded dataとは判定しません。たとえばJPEG offset 0に続くEXIF内TIFFは正常なJPEG構造であり、別の隠しfileとは扱いません。

`binwalk: embedded file signatures detected` は、通常構造として除外されない追加signatureがoffset付きで見つかったことを示します。候補であり、必ず隠しfileがあるという断定ではありません。詳細はoutput directoryの `binwalk.txt` を確認してください。

## `extract`

completed scanのfindingとtool結果を再利用して抽出します。

- Binwalk offset候補をcarvingします。
- scanで得たzsteg selectorを使ってpayloadを出力します。
- StegHide候補はempty passphrase、manual passphrase、またはstegseek wordlistを状況に応じて試します。
- text fileでは利用可能ならstegsnowを扱います。

何も候補がないscanに対してはpassword解析を尋ねません。password保護されたStegHide候補が検出された場合だけ、未指定時に次を選択させます。

```text
1) Auto   ctx password wordlistで解析
2) Manual 既知passphraseを入力
3) Skip   password解析をしない
```

### `--auto`

対話選択を省略して自動解析します。`-w, --wordlist <file>` がなければctxの `password` 推薦をpriority順に使用します。試行済みwordlistはreportへ記録します。

### `--manual`

passphraseを端末から非表示入力します。誤ったpassphraseは成功扱いにせず、抽出fileもfindingも作りません。`--wordlist`、`--no-crack` とは併用できません。

### `--no-crack`

wordlist解析を行いません。このoptionはauto modeとして扱われ、空passphraseなどpassword cracking以外の抽出可能性は処理します。

## Output directory

sourceと同じ場所に `<source>.xsteg` を作ります。

```text
oneforall.jpg.xsteg/
  report.json
  file.txt
  exiftool.txt
  binwalk.txt
  ...
  files/
    binwalk/
    zsteg/
    steghide/
```

同じsource/SHA-256のreportは再利用します。既存directoryが空なら再利用し、別reportなどと衝突する場合だけ `.xsteg.2` のような連番を使います。

## Report

`report.json` はsource path、SHA-256、MIME、mode、status、backend結果、finding、warning、抽出pathを保持します。

代表status:

- `complete`: scan/extract処理が完了
- `failed`: 必要処理が失敗

`complete` は「秘密fileが存在した」「passwordが正しかった」という意味ではありません。成果は `Findings` の `extracted` とpathで判断します。findingがなければ `Findings: none` と表示します。

```sh
xsteg ls
xsteg show 1
```

`ls [path]` はpath配下のreportをID付きで一覧表示します。`show <ID> [path]` はsource、hash、mode、status、output、finding、warning、backend結果を表示します。IDは指定した検索rootでの一覧に対応します。

## Resource上限

- backend outputのlog capture: 1 streamあたり2 MiB
- 抽出file: 1件100 MiBまで
- 抽出file数: 100件まで

上限を超える出力は失敗または打ち切りとして扱い、無制限にdiskを消費しないようにします。抽出物は信用せず、実行前にtypeと内容を確認してください。

## xlog連携

ctx workspace内で実行した場合、xsteg全体を親log、file/exiftool/binwalk/steghide/stegseek/zstegなどを子stepとして保存します。`ctx log` では親を表示し、詳細のstepsで各backend、終了状態、stdout/stderrを確認できます。

passphraseを受け取るargumentはlog上で `<redacted>` に置換します。抽出reportには自動解析で発見したpasswordが記録される場合があるため、reportの共有には注意してください。

## `doctor`

必須・任意backendのinstall状態とctx password wordlistの利用可否を表示します。期待した検出が行われない場合は最初に実行してください。

## よくある問題

- scanが遅い: payloadがないことを確認するには各backendが最後まで走るため、早期に候補を得る場合より時間がかかることがあります。
- scanなしextractで失敗する: 仕様です。先に `xsteg scan` を実行します。
- 正しいpassphraseなのに抽出されない: `doctor`、StegHide対応形式、scan finding、backend outputを確認します。
- wrong passwordなのにcompleteに見える: report mode/statusだけでなく `Findings` と抽出pathを確認します。manual failureは非0終了になります。
- Binwalkだけcandidateを出す: offsetとdescriptionを確認し、EXIFなど通常構造との区別を行います。
