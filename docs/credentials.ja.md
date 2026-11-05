# Credential providers

Retri は既存の `password`、`secret`、`${VAR}`、`RETRI_SSH_PASSWORD`、
`RETRI_SSH_SECRET` をそのまま利用できます。名前付き credential は任意機能であり、
Bitwarden、Linux keyring の導入は必須ではありません。

名前付き credential を使うと、**秘密情報をどこから取得するか**と
**どこで利用するか**を分離できます。

```yaml
credentials:
  linux-login:
    provider: prompt
    prompt: "Linux SSH password"

  linux-sudo:
    provider: prompt
    prompt: "Linux sudo password"
    cache:
      backend: session-keyring
      ttl: 8h

defaults:
  password_credential: linux-login
  secret_credential: linux-sudo
```

同じ名前付き credential を複数ホストが使う場合でも、`prompt` は1回の Retri 実行につき
1回だけ表示されます。`session-keyring` cache を有効にすると、同じ Linux ログイン
セッション内の後続の Retri 実行でも TTL が切れるまで再利用できます。

## Provider

### `prompt`

端末から非表示で入力します。

```yaml
credentials:
  common-sudo:
    provider: prompt
    prompt: "Common sudo password"
```

### `env`

環境変数から取得します。既存の環境変数方式と同じ露出特性を持ちます。

```yaml
credentials:
  common-login:
    provider: env
    ref: COMMON_LOGIN_PASSWORD
```

### `keyring`

Linux Session Keyring に既に登録されている `user` key を読み込みます。
Linux kernel APIを直接利用するため、Retriに`keyctl`コマンドは不要です。

```yaml
credentials:
  common-sudo:
    provider: keyring
    ref: retri-linux-sudo
```

### `bitwarden`

公式 Bitwarden CLI の `bw` を使って login item を取得します。
接続先は `bw config server` で決まります。
Retri 自身は vault の login / unlock を行いません。

```yaml
credentials:
  network-login:
    provider: bitwarden
    server: https://vault.bitwarden.com  # must match bw status serverUrl
    account: "BITWARDEN-USER-ID"          # bw status userId (not email)
    ref: 00000000-0000-0000-0000-000000000000
    field: password
```

`field` の既定値は `password` です。`username` と custom field 名も指定できます。

### Bitwardenの初期設定

Retriは公式の`bw` CLIを利用します。Retriを実行するWSL環境に`bw`をインストールし、
Bitwardenの外部公開URLを設定してください。

```bash
bw config server https://vault.example.com
bw login your-email@example.com
bw unlock
```

`bw unlock` が表示する Bash 用の`export BW_SESSION=...`を、Retriを実行する同じshellで
実行します。vault itemの内容を表示せず、接続先とunlock状態を確認します。

```bash
bw status
```

出力の`serverUrl`と`userId`を使います。`account`にはメールアドレスではなく`userId`を
指定してください。BitwardenにLogin itemを作成し、名前で検索してitem IDを取得できます。

```bash
bw list items --search retri
```

Retriの設定でitemを参照します。`field`の既定値は`password`で、`username`またはcustom
field名も指定できます。

```yaml
credentials:
  bitwarden-ssh:
    provider: bitwarden
    server: https://vault.example.com
    account: "00000000-0000-0000-0000-000000000000"
    ref: "BITWARDEN-ITEM-ID"
    field: password

defaults:
  password_credential: bitwarden-ssh
```

sudoやenable secretが別の場合は、別の名前付きcredentialを作成してください。他のclientで
Bitwardenのitemを変更した場合は、Retri実行前にCLI vaultを同期します。

```bash
bw sync
```

他のproviderを使うユーザーに`bw`は必要ありません。Retriがlogin、unlock、lock、logout、
syncを自動実行することもありません。

### `literal`

値を Retri の設定ファイルに直接保持します。移行やテスト用途のために用意していますが、
実運用の秘密情報では非推奨です。

```yaml
credentials:
  test-only:
    provider: literal
    value: test-password
```

## Linux keyring cache

### Cache backend の比較

**同じ UID で共有してよい通常の利用は `user-keyring` が基本です。tmux を使わない場合も選べます。**
再利用を継承セッションに結び付けたい場合は `session-keyring`、実行をまたぐ cache が不要なら
`none`（未指定時の既定値）を選びます。

| 比較項目 | `user-keyring` | `session-keyring` | `none` / 未指定 |
| --- | --- | --- | --- |
| 向いている用途 | 通常の CLI 利用、tmux、別ログインからの再利用 | 継承セッションに結び付けた再利用 | 実行ごとに取得したい場合 |
| Retri が再利用する範囲 | 同じ UID のプロセス・ログイン間 | 同じ Session Keyring を継承したプロセス間 | 1回の Retri 実行内だけ |
| tmux の別ペイン・再接続 | 同じ UID なら再利用可能 | 同じ keyring を継承し、有効な間は再利用可能 | 実行ごとに取得 |
| 親 Session Keyring の失効 | 影響を受けない | cache を利用できなくなる | 影響を受けない |
| ログアウト後 | UID のプロセス等が残っていれば TTL まで残る場合がある | PAM 等による失効・keyring の寿命に依存 | 実行をまたぐ cache はない |
| TTL | 既定8h、1s〜24h | 既定8h、1s〜24h | 設定不可 |
| 平文のディスク cache | 作らない | 作らない | 作らない |

両 keyring backend とも読み出しで TTL は延長しません。現在の Retri は両方で所有 UID に
読み取り権限を与えるため、`session-keyring` を選んでも、同じ UID の別プロセスからの
厳密な隔離やログアウト直後の確実な全消去を保証するものではありません。
`session-keyring` は既存のセッションに結び付いた動作を維持したい運用のために残しています。

保存先の仕様: [User Keyring](https://man7.org/linux/man-pages/man7/user-keyring.7.html)、
[Session Keyring](https://man7.org/linux/man-pages/man7/session-keyring.7.html)。

### Cache の設定

Cache は provider とは独立しています。

```yaml
credentials:
  network-login:
    provider: bitwarden
    server: https://vault.bitwarden.com  # must match bw status serverUrl
    account: "BITWARDEN-USER-ID"          # bw status userId (not email)
    ref: 00000000-0000-0000-0000-000000000000
    cache:
      backend: session-keyring
      ttl: 8h
```

Retri は provider にアクセスする前に cache を確認します。そのため一度
Bitwarden から取得済みであれば、ネットワーク断や Bitwarden 停止中でも
key が残っていて TTL 内であれば利用できます。

`session-keyring` と `user-keyring` は Linux のみ対応し、kernel APIを直接利用します。
`ttl` 省略時は8時間です。
ディスク上への永続 cache は作成しません。

明示的に指定した provider が失敗し、有効な cache も存在しない場合、Retri は別の
provider へ勝手に切り替えずエラーを返します。一方、credential 自体が未設定の場合は、
従来どおり非表示の fallback prompt を利用します（以下の設定で抑止できます）。

### ユーザー単位で共有する `user-keyring`

同じ UID の別ログインや tmux の別ペイン・再接続後でも再利用する場合は、
明示的に `user-keyring` を選択できます。
ログイン時の Session Keyring が失効していても、UID に対応する User Keyring を直接
利用するため、shell を作り直す必要はありません。

```yaml
credentials:
  default-sudo:
    provider: prompt
    prompt: "Default sudo password"
    cache:
      backend: user-keyring
      ttl: 8h

defaults:
  secret_credential: default-sudo
```

同じ UID の別ログインや他のプロセスからも共有でき、ログアウト後も TTL まで残る場合が
あります。通常の `session-keyring` より共有範囲が広いため、自動切り替えはしません。
所有者限定の権限、TTL（1s〜24h）、平文のディスク cache を作らない条件は共通です。
読み出しで TTL は延長しません。backend ごとに cache の識別子を分離しており、切り替えた
場合は新しい保存先で取得し直します。元の cache は元の TTL で失効します。
`--credential-cache-init`、refresh、clear は両 backend に対応します。

## パスワード入力の抑止と cache の事前作成

`prompt_password: false` で SSH パスワード、`prompt_secret: false` で sudo / enable
secret の入力を抑止します。両方とも既定値は `true` で、`defaults`、`groups`、
`device_types`、`hosts` に設定できます。通常の優先順位で継承し、上位の `true` で
再び許可できます。

```yaml
credentials:
  login:
    provider: prompt
    prompt: "SSH password"
    cache: {backend: session-keyring, ttl: 8h}
  sudo:
    provider: prompt
    prompt: "Sudo password"
    cache: {backend: session-keyring, ttl: 8h}

defaults:
  password_credential: login
  secret_credential: sudo
  prompt_password: false
  prompt_secret: false
```

credential 未設定時は入力を省略します。名前付き `prompt` provider は有効な cache が
あれば利用し、なければ入力せずエラーにします。設定済みの値、環境変数、他の provider
の利用は継続します。対象は Retri の自動実行時の認証情報入力です。
`retri <hostname>` やローカル記録セッション内の SSH / sudo 自体の対話入力は制御しません。

上記の設定では、同じ Linux セッションから先に以下を実行します。

```bash
retri -c config.yaml --credential-cache-init
retri -c config.yaml --credential-cache-init -H myserver
retri -c config.yaml --credential-cache-init -g web
retri -c config.yaml -i  # --credential-cache-init の短縮形
```

`-i` / `--credential-cache-init` は選択した SSH / sudo credential の cache を準備して終了します。
対象省略時は `defaults` の参照だけを選び、`-H`、`-g`、または1つの位置引数でホスト別の
設定を選べます。コマンド指定は不要で、設定済みのコマンドも実行しません。
SSH 接続、shell 起動、セッションログ作成、更新確認、初期設定ファイル生成は行いません。
明示的な初期化なので `prompt_password` / `prompt_secret` が `false` でも入力できます。

選択された参照のうち、`cache.backend: session-keyring` または `user-keyring` を設定した
名前付き credential だけを初期化します。cache 未設定の credential、`password` / `secret` の直接指定、
legacy 環境変数、`-p` / `-s` の上書き値は入力・取得・保存せずスキップします。
対象が1件もない場合は案内メッセージを表示し、終了コード `0` で正常終了します。
対象がある場合は Linux keyring が必要で、`prompt` provider は terminal stdin を
必要とします。設定の不備や対象 credential の取得・保存失敗は引き続きエラーです。
他の provider を選んだ場合は、その既存の依存関係・接続条件が適用されます。
秘密情報は選択した Linux keyring に保存し、平文のディスク cache は作りません。

有効な cache は再利用し、期限を延長しません。取り直す場合は
`--credential-cache-init --credential-cache-refresh`、破棄する場合は
`--credential-cache-clear` を使用します。複数 credential の途中で取得に失敗した場合、
それまでに保存した cache は残ります。`--command`、`-f`、`--update`、
`--credential-cache-clear` との同時指定はエラーです。

## セキュリティ境界

Session Keyring は、設定ファイル、プロセス引数、継承された環境変数などへの偶発的な
露出を減らします。ただし、Retri が平文パスワードを実際に利用している瞬間まで含めて、
完全な root 権限を持つ攻撃者から秘密を守るものではありません。可能な環境では SSH
password 自体を廃止し、公開鍵認証や FIDO2 を利用する方が強固です。

SSHと記録用shellからは`RETRI_SSH_PASSWORD`、`RETRI_SSH_SECRET`、`BW_*`、
`BWS_ACCESS_TOKEN`に加え、設定内のenv providerが参照する変数と、legacy/literal
credentialの展開に使う変数を除外します。`bw`には`BW_SESSION`など自身の環境を
残します。SSH agent、proxy、localeの通常設定は維持します。親プロセスに存在する
すべてのアプリケーションのsecretを検出する仕組みではありません。

## 解決順序と寿命

優先順位は`defaults < groups < device_types < hosts < CLI`です。複数groupでは
後のgroupが優先されます。`RETRI_SSH_PASSWORD` / `RETRI_SSH_SECRET`は設定未選択時
だけの補完です。legacy `${VAR}`が空に展開された場合も補完できます。同一layerでは
literalと名前付き参照は排他ですが、上位layerはどちらの形式でも上書きできます。
CLI `-p/-s`は展開しないliteralで、プロセス引数として見える可能性があります。

名前付きproviderは空文字・NUL・CR・LFを含む値を返せません。失敗時にlegacy envや
promptへ切り替えません。明示promptにはterminal stdinが必要です。一方、未設定の
credentialは非TTYでは省略し、公開鍵認証やsudo不要の自動実行を妨げません。
`${VAR}`展開の対象はlegacy password/secretとliteralの`value`です。
`ref`、`server`、`account`、`field`、promptラベルは展開しません。

取得値とエラーは名前ごとに1実行内で共有し、端末promptは直列化します。
host実行は取得済みsnapshotを使います。実行中にsession cacheのTTLが切れても、
接続済みSSHや待機中hostのsnapshotを破棄しません。TTLは後続実行の再利用期限です。

session cacheのTTLは既定8h、指定範囲1s〜24hです。読み出しても延長しません。
TTL短縮時は元の取得時刻からの経過時間にも適用します。識別子には設定ファイルの
canonical path、credential名、source、ref、field、Bitwarden profileを含めます。
literal定義の変更は保護されたpayload内で検証し、公開メタデータにはsecretのhashを
出しません。env変数や上流secretの変更だけでは期限内cacheを自動更新しません。

```bash
retri -c config.yaml --credential-cache-refresh -g production
retri -c config.yaml --credential-cache-clear
```

refreshは選択されたcacheを削除して同じsourceから取得し直します。取得失敗時に
旧値を復活させません。clearは現在の定義のcacheを削除し、providerに接続せず終了
します。削除・改名された定義の旧entryは旧TTLで失効します。cache障害はmissとして
隠さずエラーにします。別プロセス間のprompt重複排除は行わず、各snapshotは独自の
期限を持ちます。

Linux session keyringは子プロセスに継承され、PAM/service構成により範囲が異なります。
必ずしも端末や人間のログイン単位ではありません。cache keyは所有者だけに権限を
与えますが、同一ユーザーの別プロセスからの隔離は保証しません。各snapshotの専用
keyringに権限・kernel timeoutを設定してから、内部のvalue keyにsecretを公開します。
value keyにも公開前に権限、公開後にtimeoutを設定します（user key更新はtimeoutを
リセットするため）。途中終了しても外側のringの期限で保持を制限し、失敗時はrevoke
します。平文disk cacheは作りません。Go string/runtime copy、swap、crash dump、
特権によるメモリ参照は防御範囲外です。一時byte bufferは可能な範囲で消去しますが、
完全なメモリ消去は保証しません。

### Session Keyring が失効している場合

親の Session Keyring 自体が失効している場合、Retri は cache 未作成として扱わず、
パスワード入力前にエラーと復旧方法を表示します。ログアウト時の PAM の処理などで
keyring が失効し、長時間動く shell や tmux 等が古い keyring を継承している場合があります。

新しいログインセッションを使用するか、`keyctl` がある場合は手元の端末で新しい shell
を起動してください。既存の keyring を削除・失効させる操作は不要です。

```bash
keyctl session - bash
keyctl id @s
retri -c config.yaml -i -H myserver
```

`keyctl id @s` が失敗した場合はそこで止めてください。成功後は、cache の作成と後続の
Retri 実行をすべてこの新しい shell 内で行います。終了は `exit` で元の shell に戻ります。
Retri が自身のプロセスだけで keyring を作り直しても、親 shell からの後続実行とは共有
できないため、自動的な置き換えは行いません。`keyctl` はこの復旧手順用であり、通常の
Retri 実行の依存関係ではありません。

v2 cacheはこの未マージPRの旧実装が書いたentryを利用しません。旧実装を実行済みなら
keyutilsで旧`retri:credential:` keyを確認・削除してください。旧timeout設定失敗に
より期限なしentryが残っている可能性があります。

## Bitwardenの運用

`bw`はBitwarden sourceへの実アクセス時だけ必要です。`--nointeraction`、30秒timeout、
stdout上限を指定し、stderrや応答本文をエラーに出しません。login/unlock/lock/logout/
syncはRetriが管理しません。自身のCLI sessionで`BW_SESSION`をexportし、他clientの
変更が必要なら`bw sync`してください。Retri cacheの失効はCLI vaultの同期を意味
しません。

Bitwardenのsession cacheには`server`と`account`が必要です。`bw status`の`serverUrl`
と`userId`を使ってください（emailではありません）。cache miss時は接続先・account・
unlocked状態を確認してからitemを読みます。hit時は`bw`、unlock、ネットワークが不要
です。vaultのlock/logoutでRetriの独立cacheは消えないため、明示的にclearしてください。
接続先/account変更時は設定も更新すると、別cacheとして扱われます。

自動実行ログは既知credentialを受信chunk境界をまたいで伏せ、terminal描画後にも
伏せます。debugのliteral値も対象です。相手が任意変換したsecretの検出は保証しません。
対話record modeでRetriが取得していないcredentialは伏せられません。
