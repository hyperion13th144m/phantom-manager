# phantom-manager（新）設計・引き継ぎ資料

作成日: 2026-08-10
作成環境: 開発用ネイティブ Ubuntu（ホスト名 triglav / WSL ではない）

> **この文書の役割**
> 新 phantom-manager の実装は **Windows + WSL2 マシンの WSL 側 Claude Code** で行う。
> この文書は、そのセッションがコールドスタートで実装に入れるようにするための引き継ぎ資料。
> 要件・旧実装の分析・WSL2 固有の落とし穴・推奨アーキテクチャ・未決事項をまとめてある。
>
> **文中のソースへのリンクは `old/` を指している。**
> 旧 C# 実装（WinForms）は 2026-08-11 に `old/` へ退避した（§8-5 の決定）。
> 新実装はリポジトリ直下（`main.go` + `internal/` + `web/`）に置く。

---

## 1. 何を作るか

`$HOME/projects/phantom-release`（= [phantom-release](https://github.com/hyperion13th144m/phantom-release)）を
Windows + WSL2 上で運用するためのユーティリティ。

**形態: WSL2 内で動くローカル Web UI。**
WSL2 内でローカル HTTP サーバを起動し、Windows 側のブラウザから `http://localhost:<port>` で操作する。
（WSL2 の localhost forwarding により、WSL 内の `127.0.0.1:PORT` は Windows 側の `localhost:PORT` から見える）

旧 manager は **Windows ネイティブの WinForms アプリ**で、`wsl.exe` 経由で WSL を外から操縦していた。
新 manager は**逆**に、WSL の内側で動いて自分自身の環境を直接操作する。この反転が設計上の最大の差分。

### 必須機能（4つ）

| # | 機能 | 概要 |
|---|---|---|
| 1 | リポジトリ管理 | phantom-release の `clone` / `pull` |
| 2 | 取込スクリプト生成 | Windows ローカルディスク → WSL のディレクトリ（例 `~/phantom/data/src`）へファイルをコピーするスクリプトを生成 |
| 3 | `.env.docker` 生成 | `.env.docker.sample` から WSL2 向けの `.env.docker` を生成 |
| 4 | Docker Compose 操作 | `build`（**es のみ**）/ `pull` / `up -d` / `down` |

---

## 2. 前提環境

- Windows 10 / 11 + WSL2（Ubuntu）
- Docker
  - 旧 manager は **Docker Desktop for Windows**（WSL2 backend）前提で、WSL 内から
    `/mnt/c/Program Files/Docker/Docker/resources/bin/docker.exe` を叩いていた（[DockerCli.cs](old/DockerCli.cs)）
  - 新 manager は WSL 内で動くので、**WSL ネイティブの `docker` / `docker compose` を素直に使えばよい**。
    Docker Desktop 統合が有効なら `docker` は PATH 上にある（`/usr/bin/docker` → Desktop の shim）
  - ⚠ 未決: Docker Desktop 前提を維持するか、WSL 内 docker engine 直インストールも許容するか → §8
- phantom-release の既定配置: `~/phantom/phantom-release`（旧 manager の `AppPaths.DefaultReleaseDir` と同じ）
- データの既定配置: `~/phantom/data/src`（取込先）、`~/phantom/data`（展開先）

---

## 3. 旧 manager の分析

ソースは `old/` にある（GitHub の `hyperion13th144m/phantom-manager` の `main` HEAD と同一内容。
向こうではリポジトリ直下に置かれている）。.NET 8 / WinForms / 全 1560 行。

### 3.1 ファイル構成と役割

| ファイル | 役割 | 新 manager での扱い |
|---|---|---|
| [Form1.cs](old/Form1.cs) (844行) | UI 全部 + 画面ロジック全部 | **作り直し**（Web UI へ） |
| [WslCommand.cs](old/WslCommand.cs) | `wsl.exe -d Ubuntu-20.04 -- bash -lc "<script>"` のラッパ。パスの `~` 展開とシングルクォートエスケープ | **不要**（WSL 内で直接実行する）。ただし**クォート処理のロジックは移植価値あり** |
| [WslEnvironment.cs](old/WslEnvironment.cs) | `wsl.exe --version` / `--list --quiet` でディストリ検出 | **不要**（自分がその中にいる） |
| [CommandRunner.cs](old/CommandRunner.cs) | プロセス起動・stdout/stderr の行単位コールバック・タイムアウト付き quiet 実行 | **移植**。ログのストリーミング配信に必要 |
| [GitRepository.cs](old/GitRepository.cs) | clone / fetch --tags / tag 一覧 / checkout / detached HEAD 判定 | **移植**（ただし tag checkout → pull に変更、§3.3） |
| [DockerComposeClient.cs](old/DockerComposeClient.cs) | `compose up -d` / `down` / `ps --all --format json` のパース | **移植**（`--env-file .env.docker` の追加と build/pull 追加が必要） |
| [DockerCli.cs](old/DockerCli.cs) | docker.exe のパス解決（Windows 側／`/mnt/c` 側） | **不要**（WSL の `docker` を使う） |
| [MirrorBatchWriter.cs](old/MirrorBatchWriter.cs) | robocopy の `.bat` を **Shift_JIS** で生成 | **移植（要変更）**。§5 |
| [NetworkAddressProvider.cs](old/NetworkAddressProvider.cs) | vEthernet / Hyper-V を除外して実 IPv4 を選ぶ | **要再実装**。§6 が急所 |
| [NginxSslCertificateGenerator.cs](old/NginxSslCertificateGenerator.cs) | openssl でローカル CA + サーバ証明書を生成 | **保留**（新 release に対応する仕組みが無い、§3.3） |
| [ElasticsearchInitializer.cs](old/ElasticsearchInitializer.cs) | `compose exec es /init.sh -f` | **保留**（同上） |
| [release.ps1](old/release.ps1) | `dotnet publish` + zip + tag push + GitHub Release | **作り直し**（§7 の配布方式に合わせる） |
| [wsl-install.bat](old/wsl-install.bat) | 中身は `wsl --install -d Ubuntu-20.04` の 2 行 | 新方式では Windows 側の導線として別途検討 |
| [INSTALL.md](INSTALL.md) + `assets/*.jpg` | スクリーンショット付きインストール手順（33枚） | **画像は作り直しが必要**（UI が変わるため）。文章構成は流用可 |

### 3.2 旧 UI の構成（画面設計の参考）

```
┌─ phantom 全文検索システム管理 v1.0.7 ───────────────────────┐
│ ┌ 環境チェック ─┐ ┌ バージョン ──┐ ┌ データディレクトリ ─┐ │
│ │ ○ Docker      │ │ [release path]│ │ [.env 保存]         │ │
│ │ ○ Git in WSL  │ │ [clone]       │ │ [元データ選択]      │ │
│ │ ○ WSL Ubuntu  │ │ ─────         │ │ [ミラーバッチ作成]  │ │
│ │ ○ release     │ │ [tag ▼][更新] │ │ [データフォルダ開く]│ │
│ │ [再チェック]  │ │ [チェックアウト]│ │                    │ │
│ │ [DB初期化]    │ │ 現在: v...    │ │                     │ │
│ └───────────────┘ └───────────────┘ └─────────────────────┘ │
│ ┌ サービス ───────────────────────────────────────────────┐ │
│ │ [起動][SSL][停止][状態更新][SSL証明書][CA] http://…:8080 │ │
│ │ Service | State | Status | Ports  (ListView)             │ │
│ └──────────────────────────────────────────────────────────┘ │
│ ┌ ログ ────────────────────────────────────────────────────┐ │
│ │ [HH:mm:ss] ... 実行コマンドと出力を全部流す                │ │
│ └──────────────────────────────────────────────────────────┘ │
└──────────────────────────────────────────────────────────────┘
```

**踏襲すべき良い点:**
- 実行したコマンドと出力を**すべてログ枠に流す**（`> wsl.exe -d ... -- bash -lc ...` の形で）。
  トラブル時にユーザーがログを貼れば原因が分かる。Web UI では **SSE でストリーミング**する。
- **状態に応じたボタンの有効/無効制御**（[Form1.cs:720](old/Form1.cs:720) `SetBusy`）。
  サービス起動中は `.env` 保存やタグ切替を無効化して、実行中サービスとの競合を防いでいる。
  この排他ロジックはそのまま活きるので、条件を移植すること。
- 起動時に自動で全チェックを走らせる（`Shown += RefreshAllAsync`）。

### 3.3 旧 manager にあって新要件に無い機能

新要件（§1 の4つ）に含まれないが、旧 manager にはあった機能。**扱いは未決 → §8。**

| 機能 | 状況 |
|---|---|
| タグ選択 + チェックアウト | 新要件は「clone / pull」なので**バージョン固定運用をやめる**という意図に読める。要確認 |
| SSL 証明書生成 / CA ダウンロード | 新 release に `docker-compose.secure.yml` が**存在しない**。対応する仕組みが無いので現状は実装不可 |
| データベース初期化 | 旧は `compose exec es /init.sh -f`。新 release の `infra/es/Dockerfile` に `init.sh` は**無い**（マッピング適用は panther 側に移った模様）。現状は実装不可 |
| 環境チェック（Docker / Git / WSL / release の有無） | 要件外だが**残す価値が高い**。WSL 内なら `docker info` / `git --version` / ディレクトリ存在確認だけで済み、実装コストも低い |
| サービス一覧表示（compose ps） | 要件外だが up/down する以上は必要。**残すべき** |

---

## 4. phantom-release 側の変更点（旧 manager が前提にしていたものとの差分）

**ここが一番大きい。旧 manager の `.env` 生成ロジックはそのままでは使えない。**

| | 旧（旧 manager が前提） | 新（現 phantom-release） |
|---|---|---|
| 設定ファイル | `env.sample` → `.env` | `.env.docker.sample` → **`.env.docker`** |
| compose への渡し方 | 暗黙の `.env` | **`docker compose --env-file .env.docker ...`（明示必須）** |
| 取込元 | `SRC_DIR=./var/internet-app-data`（リポジトリ相対の固定値） | **`PHANTOM_SRC_DIR`（絶対パス、既定値なし）** |
| 展開先 | 同上 | **`PHANTOM_DATA_DIR`（絶対パス、既定値なし）** |

`.env.docker` の要点（`phantom-release/.env.docker.sample` 参照）:

```
PHANTOM_SRC_DIR=/mnt/disk/jpodata/src        # crow が読む。:ro でマウント。既定値なし＝必須
PHANTOM_HTML_SRC_DIR=/mnt/disk/jpodata/html-src  # 未設定なら PHANTOM_SRC_DIR にフォールバック
PHANTOM_DATA_DIR=/var/lib/phantom/data       # 全サービスが読み書き。既定値なし＝必須
PHANTOM_HTTP_PORT=8080                       # ホストに出るのは nginx のこのポートだけ
PHANTOM_PUBLIC_URL=http://localhost:8080     # joker → fox のリンク生成に使う
ES_PASSWORD / ES_JAVA_OPTS / *_MEM_LIMIT ...
```

compose 内での使われ方（`docker-compose.yml`）:

```
35:  - ${PHANTOM_SRC_DIR}:/src-dir:ro                          (crow)
36:  - ${PHANTOM_DATA_DIR}:/data-dir                           (crow)
58,83,107,141,170: ${PHANTOM_DATA_DIR}:/data-dir              (queen/noir/violet/cendrillon/panther)
140:  - ${PHANTOM_HTML_SRC_DIR:-${PHANTOM_SRC_DIR}}:/html-src-dir:ro  (cendrillon)
194:  - ${PHANTOM_DATA_DIR}:/data-dir:ro                       (mona)
325:  - ${PHANTOM_HTTP_PORT:-8080}:8080                        (nginx)
```

**build 対象が `es` だけ**なのは compose ファイルの実態と一致している。
`build:` を持つサービスは `es` のみ（`context: ./infra/es`、image 名 `phantom-elasticsearch`）。
他 12 サービスはすべて ghcr.io の digest 固定 `image:`。

```bash
docker compose --env-file .env.docker build es
docker compose --env-file .env.docker pull      # es 以外を取得
docker compose --env-file .env.docker up -d
docker compose --env-file .env.docker down
```

---

## 5. 機能 2「取込スクリプト生成」の設計

**Windows のローカルディスク（例 `D:\jpodata`）→ WSL の `~/phantom/data/src` へのコピー。**

### 旧実装の方式（[MirrorBatchWriter.cs](old/MirrorBatchWriter.cs)）

Windows 側で実行する `.bat` を生成し、`robocopy` で **Windows → `\\wsl.localhost\<distro>\...`** へミラーする。

```bat
@echo off
set "ORIG=D:\jpodata"
set "DATA_DIR=\\wsl.localhost\Ubuntu-20.04\home\user\phantom\phantom-release\var\internet-app-data"
robocopy "%ORIG%" "%DATA_DIR%" "*AAA.JWX" "*AAA.JPC" "*NNF.JWX" "*NNF.JPC" "*AFM.XML" "*NFM.XML" /E /LOG:"...\mirror.log" /TEE
exit /b %ERRORLEVEL%
```

ポイント:
- **拡張子フィルタが業務仕様**: `*AAA.JWX` `*AAA.JPC` `*NNF.JWX` `*NNF.JPC` `*AFM.XML` `*NFM.XML`
  （インターネット出願ソフトの電子データのうち必要なものだけ）。**この 6 パターンは必ず引き継ぐこと。**
  → ただし新実装では **HTM / GIF / JPG を追加する**（§5「新実装で変わる点」4 を参照）
- **Shift_JIS で書き出している**（`Encoding.GetEncoding("Shift_JIS")`）。
  日本語パスを含む `.bat` を `cmd.exe` が正しく読むために必須。UTF-8 で書くと文字化けする。
- `robocopy /E`（サブディレクトリ込み、空ディレクトリも）、`/LOG` + `/TEE` でログ両出し。
- ドライブ直下（`D:\`）が指定された場合の正規化処理あり（`D:\` → `D:\.`）。

### 新実装で変わる点

1. **生成側が WSL 内になる。** `.bat` を WSL のファイルシステム上に書き出し、
   ユーザーは Windows のエクスプローラからダブルクリックして実行する。
   → 書き出し先は `\\wsl.localhost\<distro>\...` から見える場所（WSL の任意の場所でよい）
   → **`.bat` の中身は Windows パス表現**であることに注意（生成する側が Linux でも中身は Windows 用）
2. **コピー先が `PHANTOM_SRC_DIR`（`~/phantom/data/src`）になる。**
   `.bat` の `DATA_DIR` には `\\wsl.localhost\<distro>\home\<user>\phantom\data\src` を書く。
   → distro 名と Linux 絶対パスから UNC を組む処理は [Form1.cs:668](old/Form1.cs:668) `ToWslUncPath` を移植。
   → **distro 名の取得方法が変わる**（§6-2）
3. **コピー対象に HTM / GIF / JPG を追加する（cendrillon 対応）。**
   cendrillon の実装により、XML が残っておらず **HTML + 画像しかない文書も取り込めるようになった**。
   これに合わせてコピー対象を **6 パターン → 9 パターン**に拡張する。

   ```bat
   robocopy "%ORIG%" "%DATA_DIR%" ^
     "*AAA.JWX" "*AAA.JPC" "*NNF.JWX" "*NNF.JPC" "*AFM.XML" "*NFM.XML" ^
     "*.HTM" "*.GIF" "*.JPG" ^
     /E /LOG:"...\mirror.log" /TEE
   ```

   - 既存 6 パターンは「接尾辞 + 拡張子」指定（`*AAA.JWX`）だが、追加分は**拡張子のみ**の指定（`*.HTM`）。
     robocopy のファイルパターンはどちらも同じ扱いなので混在して問題ない
   - robocopy のパターンは**大文字小文字を区別しない**ので、`.htm` / `.jpg` の小文字も拾える
   - ⚠ `*.HTM` は `*.HTML` を**拾わない**（robocopy のワイルドカードは `*.HTM` を「拡張子が HTM」と解釈する）。
     取込元に `.html` が存在しうるなら `"*.HTML"` も足すこと → §8-9 で要確認
   - ⚠ **配置先の分離をどうするか**: compose では HTML 由来の文書を cendrillon が
     `${PHANTOM_HTML_SRC_DIR:-${PHANTOM_SRC_DIR}}` から `:ro` で読む（docker-compose.yml:140）。
     **9 パターンすべてを `~/phantom/data/src` の 1 箇所にコピーするなら、`PHANTOM_HTML_SRC_DIR` は
     未設定のままでよい**（`PHANTOM_SRC_DIR` にフォールバックする）。これが最も単純。
     分離したい場合のみ `.env.docker` に `PHANTOM_HTML_SRC_DIR` を書き、robocopy も 2 本立てにする → §8-9

4. **コピー元（Windows パス）の入力手段が無い。**
   旧は WinForms の `FolderBrowserDialog` でフォルダを選ばせていた。Web UI にはこれが無い。
   → **候補 A**: `/mnt/` 配下をサーバ側で走査してディレクトリツリーを自前で出す（`/mnt/c`, `/mnt/d` …）。
     選んだ Linux パスを `wslpath -w` で Windows パスへ変換して `.bat` に書く。**これを推奨**
   → 候補 B: テキスト入力で `D:\jpodata` を直接打たせる（最も簡単だが入力ミスに弱い）
   → 候補 C: `powershell.exe` 経由で Windows のフォルダ選択ダイアログを出す（体験は良いが実装が重く、フォーカス問題が出る）

### robocopy でなく WSL 側でコピーする案について

`cp -r /mnt/d/jpodata/... ~/phantom/data/src` を WSL 内で直接実行することも技術的には可能。
ただし **`/mnt/` 経由（9p/drvfs）の I/O は極端に遅く**、数万ファイル規模だと実用にならない。
旧実装が Windows 側の robocopy にしているのはこの理由と思われるので、**robocopy 方式を維持すべき**。
（`.bat` を生成してユーザーに実行させる、という要件の書き方もこれと整合している）

---

## 6. WSL2 依存ポイント（実装の急所）

**この開発用 Ubuntu では一切検証できなかった箇所。WSL 側で必ず実機確認すること。**

### 6-1. `/mnt/c` などの Windows ドライブマウント
- 存在確認: `ls /mnt/`、マウント種別は `mount | grep drvfs`
- ドライブ一覧の列挙方法を決めること（`/mnt/` の直下を読むのが素直。ただし WSL の `automount` 設定で
  マウントポイントが変わりうる。`/etc/wsl.conf` の `[automount] root=` を見るのが厳密）

### 6-2. パス変換
- `wslpath -w /mnt/d/foo` → `D:\foo`、`wslpath -u 'D:\foo'` → `/mnt/d/foo`
- **`wslpath` は WSL 専用コマンド**。この Ubuntu には無いので未検証。
- distro 名の取得: 旧は Windows 側から `wsl.exe --list --quiet` で取っていた。WSL 内からは
  **`$WSL_DISTRO_NAME` 環境変数**が使える（これが最も簡単）。フォールバックとして
  `wslpath -w /` の結果（`\\wsl.localhost\<distro>\`）からパースする手もある。
- 旧 manager は distro を `Ubuntu-20.04` に**ハードコード**していた（[WslEnvironment.cs:5](old/WslEnvironment.cs:5)）。
  新 manager では `$WSL_DISTRO_NAME` で動的に取ること。

### 6-3. WSL2 のネットワーク（旧 `NetworkAddressProvider` の置き換え）— **最重要**

旧 manager は **Windows プロセスとして** `NetworkInterface.GetAllNetworkInterfaces()` を呼び、
`vEthernet` / `Hyper-V` を名前で除外して**Windows ホストの LAN IP** を得ていた
（[NetworkAddressProvider.cs](old/NetworkAddressProvider.cs)）。これを `http://<IP>:8080/` の表示と
SSL 証明書の CN/SAN に使っていた。

**新 manager は WSL 内で動くので、この方法が使えない。** WSL 内で `hostname -I` を叩くと
WSL の仮想 NIC の `172.x.x.x` が返り、これは LAN の他 PC からは到達できない。

対処の選択肢:
- **A. `localhost` で済ませる**: 同一 PC のブラウザから使うだけなら `PHANTOM_PUBLIC_URL=http://localhost:8080`
  で十分（`.env.docker.sample` の既定値もこれ）。**まずはこれで良いはず**
- **B. Windows ホストの LAN IP が必要な場合**（LAN 内の他 PC から phantom を使う）:
  - `powershell.exe -NoProfile -Command "..."` を WSL から呼んで Windows 側の IP を取る
  - WSL2 のポートは既定で Windows の `localhost` にしか出ないため、**LAN 公開には
    `netsh interface portproxy` か WSL2 の mirrored networking モードが別途必要**。
    旧 manager が `http://<LAN IP>:8080/` を表示できていたのは Docker Desktop が
    Windows 側でポートを公開していたため。**WSL 内 docker engine 直だとこの前提が崩れる** → §8 と関連
- ⚠ **`PHANTOM_PUBLIC_URL` の値をどう決めるかは、この選択に直結する。**

### 6-4. Windows 側アプリの起動（interop）
- `explorer.exe .` でフォルダを開く、`explorer.exe http://localhost:7777` で既定ブラウザを開く
- `wslview`（wslu パッケージ）があればそちらの方が行儀が良い
- 旧の「データフォルダを開く」（[Form1.cs:637](old/Form1.cs:637)）に相当
- ⚠ interop が無効化されている環境（`/etc/wsl.conf` の `[interop] enabled=false`）を考慮し、
  **失敗しても致命的にならない設計**にすること（URL を表示してユーザーにコピーさせる導線を残す）

### 6-5. パーミッション（uid/gid 1000）
`.env.docker.sample` に明記あり:
> コンテナは uid=gid=1000（phantom / node）で動くので、書き込み先の PHANTOM_DATA_DIR は 1000 が書けるようにしておく

**WSL2 の既定ユーザーは uid=1000 なので、`~/phantom/data` 配下なら sudo 不要でそのまま動く。**
これはサンプルの `/var/lib/phantom/data`（`sudo install -d -o 1000 -g 1000` が必要）より WSL では素直。
→ **`.env.docker` 生成時の既定値は `$HOME/phantom/data` 系にすべき**（要件の `~/phantom/data/src` と一致）。

⚠ 落とし穴: **bind mount 先のディレクトリが存在しないと Docker が root 所有で自動生成し、
uid 1000 のコンテナが書けなくなる。** `up` の前に manager 側で `mkdir -p` しておくこと
（旧 manager も `EnsureDataDirAsync` で同じことをしていた、[Form1.cs:432](old/Form1.cs:432)）。

### 6-6. localhost forwarding
WSL 内の `127.0.0.1:PORT` が Windows の `localhost:PORT` から見えること — これが Web UI 方式の大前提。
既定で有効だが `.wslconfig` の `localhostForwarding=false` で切れる。**実機で最初に確認すること。**
バインドは `127.0.0.1` にする（`0.0.0.0` は不要かつ LAN に晒すリスク）。

---

## 7. 推奨アーキテクチャ

### 7.1 言語・スタック: **Go（単一バイナリ）を推奨**

理由:
- manager は**ブートストラップツール**。phantom がまだ無い、まっさらな WSL2 Ubuntu で
  最初に動く必要がある。**ランタイム依存ゼロ**であることの価値が非常に大きい
- 配布が `curl` + `chmod +x` の 2 手で終わる（旧の zip 展開 + exe 相当の手軽さを維持できる）
- 静的ファイル（HTML/CSS/JS）は `embed.FS` でバイナリに埋め込める
- ログのストリーミング（SSE）と外部プロセスの起動が標準ライブラリだけで書ける
- クロスコンパイルできるので、**最悪この開発用 Ubuntu でもビルドは通る**（実行検証はできないが）

代替案:
- **Python + FastAPI/uvicorn**: phantom 本体が uv workspace の Python なので環境的には馴染む。
  ただし新規 WSL2 に `uv`/venv を入れる手間が増える（ブートストラップツールとしては減点）
- **Node + Astro**: joker/fox が Astro なので統一感はあるが、Node の導入が前提になり同様に減点

### 7.2 構成

```
phantom-manager (単一バイナリ)
  └ 127.0.0.1:<port> で HTTP サーバ起動
      ├ GET  /                     … UI（embed した HTML/CSS/JS）
      ├ GET  /api/status           … 環境チェック（docker / git / release / .env.docker の有無）
      ├ GET  /api/events           … SSE。実行中コマンドの stdout/stderr を行単位で push
      ├ POST /api/repo/clone|pull
      ├ POST /api/env              … .env.docker 生成
      ├ GET  /api/browse?path=     … /mnt 配下のディレクトリ列挙（取込元選択用）
      ├ POST /api/mirror-script    … .bat 生成
      ├ GET  /api/compose/ps
      └ POST /api/compose/build|pull|up|down
```

- 起動したら `http://127.0.0.1:<port>` を stdout に出し、可能なら `explorer.exe`/`wslview` で自動オープン
- **長時間コマンド（`pull`, `build`, `up`）は非同期実行 + SSE でログを流す。**
  旧のログ枠の体験をそのまま Web に移す。`> docker compose --env-file .env.docker up -d` のように
  **実行コマンド自体もログに出す**こと（旧 `CommandRunner` と同じ）
- **同時実行の排他**: 実行中は他の操作を受け付けない（旧 `SetBusy` 相当）。サーバ側で 1 本のミューテックスを持つ

### 7.3 移植時に活かす旧コードの知見

- **シェルクォート**: [WslCommand.cs:52](old/WslCommand.cs:52) の `Quote()`（`'` → `'"'"'`）。
  ただし WSL 内で動くなら `exec.Command` に引数配列で渡せばシェルを介さずに済むので、
  **可能な限りシェルを経由しない**設計にした方が安全
- **`~` の展開**: 旧は `~` → `$HOME` に置換していた。Go では `os.UserHomeDir()` で解決する
- **compose ps の JSON パース**: [DockerComposeClient.cs:31](old/DockerComposeClient.cs:31)。
  `--format json` は **1 行 1 サービスの JSON Lines**（配列ではない）。`Ports` が空なら
  `Publishers` 配列から `published->target/protocol` を組み立てるフォールバックが実装済み。**この処理は移植価値が高い**
- **ボタンの有効/無効条件**: [Form1.cs:720](old/Form1.cs:720) をそのまま条件表に起こして移植

---

## 8. 未決事項（→ 2026-08-11 に全件決着）

**決定（実機確認と方針判断の結果）**

| # | 決定 | 根拠 |
|---|---|---|
| 1 | **Docker Desktop for Windows の WSL 統合**を前提とする | LAN 公開の前提を維持するため。実機で Server 29.7.2 / Compose v2.16.0 を確認 |
| 2 | clone / pull に加えて**タグ選択・チェックアウトも維持** | 実機の `~/phantom/phantom-release` が v1.0.36 の detached HEAD で、固定運用が実際に使われていた |
| 3 | SSL 証明書生成は**廃止** | phantom-release v2.0.3 に `docker-compose.secure.yml` が存在しない |
| 4 | データベース初期化は**廃止** | 同上、`infra/es` に `init.sh` が無い |
| 5 | 新実装をリポジトリ直下、**旧 C# は `old/` へ退避** | 同一リポジトリで履歴を継続する |
| 6 | 取込元選択は **PowerShell 経由の Windows 名前空間ブラウズ**（当初案の `/mnt` 走査は却下）＋ Windows パス直接入力 | ネットワークドライブが `/mnt` に出ないため。§5 の「新実装で変わる点 4」を参照 |
| 7 | `PHANTOM_PUBLIC_URL` は `localhost` 既定、UI から Windows の LAN IP に差し替え可 | |
| 8 | INSTALL.md のスクリーンショット撮り直しは新 UI 完成後の別タスク | |
| 9 | **11 パターンを 1 箇所（`~/phantom/data/src`）へ**。`PHANTOM_HTML_SRC_DIR` は書かない | `*.HTML` と `*.PNG` を追加。分離はしない |

**実機で確認した前提（開発機 = Windows + WSL2 / distro 名 `Ubuntu-20.04` / 中身は Ubuntu 24.04）**

- localhost forwarding は**有効**。Windows 側の `Invoke-WebRequest http://localhost:7777` が 200 を返すことを確認済み（§6-6 の大前提をクリア）
- `/etc/wsl.conf` に **`[interop] appendWindowsPath = false`** がある。interop 自体は有効だが
  **`explorer.exe` / `powershell.exe` は PATH に無い**ので、絶対パスで解決すること（`internal/wslenv`）
- `explorer.exe` は成功しても**終了コード 1 を返す**。終了コードで成否を判定しないこと
- `/mnt` にはローカルドライブしか出ない。ネットワークドライブ（`P:` = `\\192.168.11.250\patent-bi`）は
  `/mnt/p` が存在せず、`/mnt/j` はマウントポイントだけで**空**だった
- PowerShell の出力は **CP932** なので、`[Console]::OutputEncoding=[Text.Encoding]::UTF8` +
  `ConvertTo-Json -Compress` で受けること。`Format-Table` は値が切り詰められるので使わない

---

### 8-旧. 当初の未決事項（記録として残す）

1. **Docker の前提**: Docker Desktop for Windows（WSL2 backend）必須か、WSL 内 docker engine 直も許すか。
   → §6-3 の LAN 公開の可否と、環境チェックの実装に影響する
2. **バージョン固定運用をやめるのか**: 新要件は `clone` / `pull` のみ。旧のタグ選択 + チェックアウトは廃止でよいか。
   （廃止すると常に main 追従になる。運用上それでよいか要確認）
3. **SSL 対応**: 新 phantom-release には `docker-compose.secure.yml` が無いので**現状は実装不可**。
   phantom-release 側に手を入れるのか、機能ごと落とすのか
4. **データベース初期化**: 同様に新 release に `es/init.sh` が無い。落としてよいか
5. **リポジトリ運用**: 新 manager は既存の `hyperion13th144m/phantom-manager` に入れるのか（その場合
   旧 C# ソースはブランチ/タグに退避）、別リポジトリを立てるのか
6. **取込元フォルダの選択 UI**: §5 の候補 A / B / C のどれにするか（推奨は A）
7. **`PHANTOM_PUBLIC_URL`**: `localhost` 固定でよいか、LAN 内の他 PC からのアクセスを想定するか
8. **INSTALL.md のスクリーンショット**: UI が変わるので 33 枚の撮り直しが必要。新 manager 完成後の別タスク
9. **HTML 取込の扱い**（§5-3 関連）:
   - 取込元に `.html`（4文字拡張子）が存在するか。あるなら robocopy に `"*.HTML"` も足す
   - `.PNG` など GIF/JPG 以外の画像形式が含まれる可能性はあるか
   - 電子データ（JWX/JPC/XML）と HTML + 画像を**同じディレクトリに混ぜてよいか**。
     混ぜるなら `PHANTOM_HTML_SRC_DIR` は未設定でよい。分けるなら `.env.docker` 生成と
     robocopy 生成の両方を 2 系統にする必要がある

---

## 9. WSL 側セッションの立ち上げ手順

```bash
git clone -b design/new-manager https://github.com/hyperion13th144m/phantom-manager ~/projects/phantom-manager
```

※ `main` は旧 C# 実装そのもの。`design/new-manager` ブランチはそれに **この `DESIGN.md` を足しただけ**で、
コードには一切手を入れていない。実装はこのブランチから始める（または新しくブランチを切る）。

確認すべき前提（実機で最初に叩く）:

```bash
echo "$WSL_DISTRO_NAME"; ls /mnt/; wslpath -w /; docker version; docker compose version; git --version
```

リポジトリと参照先:
- phantom-release: https://github.com/hyperion13th144m/phantom-release
- 既定の配置: `~/phantom/phantom-release`、データは `~/phantom/data`（`src` が取込先）
