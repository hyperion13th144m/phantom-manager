using System.Diagnostics;
namespace PhantomManager;

public partial class Form1 : Form
{
    private const string FixedEnvSrcDir = "./var/internet-app-data";
    private readonly TextBox _releasePathBox = new();
    private readonly TextBox _origDirBox = new();
    private readonly TextBox _logBox = new();
    private readonly ComboBox _tagBox = new();
    private readonly ListView _serviceList = new();
    private readonly Label _dockerStatus = new();
    private readonly Label _gitStatus = new();
    private readonly Label _wslStatus = new();
    private readonly Label _repoStatus = new();
    private readonly Label _checkoutStatus = new();
    private readonly LinkLabel _serviceUrlLink = new();
    private readonly Button _upButton = new();
    private readonly Button _downButton = new();
    private readonly Button _refreshServicesButton = new();
    private readonly Button _createSslCertificateButton = new();
    private readonly Button _downloadCaCertificateButton = new();
    private readonly CheckBox _sslCheckBox = new();
    private readonly Button _fetchTagsButton = new();
    private readonly Button _checkoutButton = new();
    private readonly Button _saveEnvButton = new();
    private readonly Button _selectOrigButton = new();
    private readonly Button _createMirrorBatchButton = new();
    private readonly Button _openDataDirButton = new();
    private readonly Button _refreshChecksButton = new();
    private readonly Button _initializeDatabaseButton = new();
    private readonly Button _cloneButton = new();
    private readonly Button _installUbuntuButton = new();
    private bool _anyServiceRunning;
    private bool _ubuntuInstalled;

    public Form1()
    {
        InitializeComponent();
        BuildUi();
        EnsureLogDir();
        Shown += async (_, _) => await RefreshAllAsync();
    }

    private string ReleaseDir => _releasePathBox.Text.Trim();
    private string EnvPath => $"{ReleaseDir.TrimEnd('/', '\\')}/.env";
    private string EnvSamplePath => $"{ReleaseDir.TrimEnd('/', '\\')}/env.sample";

    private void BuildUi()
    {
        Text = "phantom-manager";
        MinimumSize = new Size(1040, 980);
        Size = new Size(1180, 1040);
        StartPosition = FormStartPosition.CenterScreen;
        Font = new Font("Yu Gothic UI", 9F, FontStyle.Regular, GraphicsUnit.Point);

        var root = new TableLayoutPanel
        {
            Dock = DockStyle.Fill,
            ColumnCount = 1,
            RowCount = 5,
            Padding = new Padding(16),
        };
        root.RowStyles.Add(new RowStyle(SizeType.AutoSize));
        root.RowStyles.Add(new RowStyle(SizeType.AutoSize));
        root.RowStyles.Add(new RowStyle(SizeType.Percent, 55));
        root.RowStyles.Add(new RowStyle(SizeType.Percent, 45));
        root.RowStyles.Add(new RowStyle(SizeType.AutoSize));
        Controls.Add(root);

        root.Controls.Add(BuildHeader(), 0, 0);
        root.Controls.Add(BuildActions(), 0, 1);
        root.Controls.Add(BuildServices(), 0, 2);
        root.Controls.Add(BuildLog(), 0, 3);
        root.Controls.Add(BuildFooter(), 0, 4);
    }

    private Control BuildHeader()
    {
        var panel = new TableLayoutPanel
        {
            Dock = DockStyle.Top,
            AutoSize = true,
            ColumnCount = 1,
            Padding = new Padding(0, 0, 0, 10),
        };
        panel.ColumnStyles.Add(new ColumnStyle(SizeType.AutoSize));

        var title = new Label
        {
            Text = "phantom 全文検索システム管理",
            Font = new Font(Font.FontFamily, 14F, FontStyle.Bold),
            AutoSize = true,
            Padding = new Padding(0, 0, 24, 0),
        };

        panel.Controls.Add(title, 0, 0);
        return panel;
    }

    private Control BuildActions()
    {
        var grid = new TableLayoutPanel
        {
            Dock = DockStyle.Top,
            AutoSize = false,
            Height = 360,
            MinimumSize = new Size(0, 360),
            ColumnCount = 3,
            Padding = new Padding(0, 0, 0, 10),
        };
        grid.ColumnStyles.Add(new ColumnStyle(SizeType.Percent, 34));
        grid.ColumnStyles.Add(new ColumnStyle(SizeType.Percent, 33));
        grid.ColumnStyles.Add(new ColumnStyle(SizeType.Percent, 33));

        grid.Controls.Add(BuildStatusPanel(), 0, 0);
        grid.Controls.Add(BuildVersionPanel(), 1, 0);
        grid.Controls.Add(BuildEnvironmentPanel(), 2, 0);
        return grid;
    }

    private Control BuildStatusPanel()
    {
        var panel = NewGroup("環境チェック");
        var body = NewVertical();
        panel.Controls.Add(body);

        ConfigureStatusLabel(_dockerStatus);
        ConfigureStatusLabel(_gitStatus);
        ConfigureStatusLabel(_wslStatus);
        ConfigureStatusLabel(_repoStatus);
        _dockerStatus.Text = "Docker Desktop for Windows: 確認中";
        _gitStatus.Text = "Git for Windows: 確認中";
        _wslStatus.Text = "WSL Ubuntu-20.04: 確認中";
        _repoStatus.Text = "phantom-release: 確認中";
        body.Controls.Add(_dockerStatus);
        body.Controls.Add(_gitStatus);
        body.Controls.Add(_wslStatus);
        body.Controls.Add(_repoStatus);

        _refreshChecksButton.Text = "再チェック";
        _refreshChecksButton.AutoSize = true;
        _refreshChecksButton.Click += async (_, _) => await RefreshAllAsync();
        body.Controls.Add(_refreshChecksButton);

        _installUbuntuButton.Text = "Ubuntu-20.04 インストール";
        _installUbuntuButton.AutoSize = true;
        _installUbuntuButton.Click += async (_, _) => await InstallUbuntuAsync();
        body.Controls.Add(_installUbuntuButton);

        _initializeDatabaseButton.Text = "データベース初期化";
        _initializeDatabaseButton.AutoSize = true;
        _initializeDatabaseButton.Click += async (_, _) => await InitializeDatabaseAsync();
        body.Controls.Add(_initializeDatabaseButton);
        return panel;
    }

    private Control BuildEnvironmentPanel()
    {
        var panel = NewGroup("データディレクトリ");
        var body = new TableLayoutPanel
        {
            Dock = DockStyle.Top,
            AutoSize = true,
            RowCount = 6,
            ColumnCount = 2,
        };
        body.RowStyles.Add(new RowStyle(SizeType.AutoSize));
        body.RowStyles.Add(new RowStyle(SizeType.AutoSize));
        body.RowStyles.Add(new RowStyle(SizeType.AutoSize));
        body.RowStyles.Add(new RowStyle(SizeType.AutoSize));
        body.RowStyles.Add(new RowStyle(SizeType.AutoSize));
        body.RowStyles.Add(new RowStyle(SizeType.AutoSize));
        body.ColumnStyles.Add(new ColumnStyle(SizeType.Percent, 100));
        body.ColumnStyles.Add(new ColumnStyle(SizeType.AutoSize));
        panel.Controls.Add(body);

        _origDirBox.Anchor = AnchorStyles.Left | AnchorStyles.Right;
        _selectOrigButton.Text = "元データ選択";
        _selectOrigButton.AutoSize = true;
        _selectOrigButton.Click += (_, _) =>
        {
            using var dialog = new FolderBrowserDialog
            {
                Description = "ミラー元のデータディレクトリを選択してください",
                SelectedPath = Directory.Exists(_origDirBox.Text) ? _origDirBox.Text : Environment.GetFolderPath(Environment.SpecialFolder.MyDocuments),
                UseDescriptionForTitle = true,
            };
            if (dialog.ShowDialog(this) == DialogResult.OK)
            {
                _origDirBox.Text = dialog.SelectedPath;
            }
        };

        _saveEnvButton.Text = ".env 保存";
        _saveEnvButton.AutoSize = true;
        _saveEnvButton.Click += async (_, _) => await RunBusyAsync(async () =>
        {
            await SaveEnvAsync();
            AppendLog($".env を保存しました: {EnvPath}");
        });

        _createMirrorBatchButton.Text = "ミラーバッチ作成";
        _createMirrorBatchButton.AutoSize = true;
        _createMirrorBatchButton.Click += async (_, _) => await RunBusyAsync(async () =>
        {
            await CreateMirrorBatchAsync();
            AppendLog($"ミラーバッチを作成しました: {AppPaths.MirrorBatPath}");
        });

        _openDataDirButton.Text = "データフォルダを開く";
        _openDataDirButton.AutoSize = true;
        _openDataDirButton.Click += async (_, _) => await RunBusyAsync(OpenDataDirAsync);

        body.Controls.Add(_saveEnvButton, 0, 0);
        body.SetColumnSpan(_saveEnvButton, 2);
        body.Controls.Add(NewSpacer(10), 0, 1);
        body.SetColumnSpan(body.GetControlFromPosition(0, 1)!, 2);
        body.Controls.Add(_origDirBox, 0, 2);
        body.Controls.Add(_selectOrigButton, 1, 2);
        body.Controls.Add(_createMirrorBatchButton, 0, 3);
        body.SetColumnSpan(_createMirrorBatchButton, 2);
        body.Controls.Add(NewSpacer(10), 0, 4);
        body.SetColumnSpan(body.GetControlFromPosition(0, 4)!, 2);
        body.Controls.Add(_openDataDirButton, 0, 5);
        body.SetColumnSpan(_openDataDirButton, 2);
        return panel;
    }

    private Control BuildVersionPanel()
    {
        var panel = NewGroup("バージョン");
        var body = new TableLayoutPanel
        {
            Dock = DockStyle.Top,
            AutoSize = true,
            RowCount = 6,
            ColumnCount = 2,
        };
        body.RowStyles.Add(new RowStyle(SizeType.AutoSize));
        body.RowStyles.Add(new RowStyle(SizeType.AutoSize));
        body.RowStyles.Add(new RowStyle(SizeType.AutoSize));
        body.RowStyles.Add(new RowStyle(SizeType.AutoSize));
        body.RowStyles.Add(new RowStyle(SizeType.AutoSize));
        body.RowStyles.Add(new RowStyle(SizeType.AutoSize));
        body.ColumnStyles.Add(new ColumnStyle(SizeType.Percent, 100));
        body.ColumnStyles.Add(new ColumnStyle(SizeType.AutoSize));
        panel.Controls.Add(body);

        _releasePathBox.Text = AppPaths.DefaultReleaseDir;
        _releasePathBox.Anchor = AnchorStyles.Left | AnchorStyles.Right;
        var defaultPathButton = NewButton("既定値");
        defaultPathButton.Click += (_, _) =>
        {
            _releasePathBox.Text = AppPaths.DefaultReleaseDir;
            _ = RefreshAllAsync();
        };

        _cloneButton.Text = "clone";
        _cloneButton.AutoSize = true;
        _cloneButton.Click += async (_, _) => await RunBusyAsync(async () =>
        {
            await ReleaseRepository().CloneAsync("https://github.com/hyperion13th144m/phantom-release", AppendLog);
            await RefreshAllAsync();
            await EnsureDataDirAsync();
        });

        _tagBox.DropDownStyle = ComboBoxStyle.DropDownList;
        _tagBox.Anchor = AnchorStyles.Left | AnchorStyles.Right;
        ConfigureStatusLabel(_checkoutStatus);
        _checkoutStatus.MaximumSize = new Size(420, 0);
        _checkoutStatus.Text = "チェックアウト状態: 確認中";

        _fetchTagsButton.Text = "バージョン一覧の更新";
        _fetchTagsButton.AutoSize = true;
        _fetchTagsButton.Click += async (_, _) => await RunBusyAsync(FetchTagsAsync);

        _checkoutButton.Text = "チェックアウト";
        _checkoutButton.AutoSize = true;
        _checkoutButton.Click += async (_, _) => await RunBusyAsync(CheckoutSelectedTagAsync);

        body.Controls.Add(_releasePathBox, 0, 0);
        body.Controls.Add(defaultPathButton, 1, 0);
        body.Controls.Add(_cloneButton, 0, 1);
        body.SetColumnSpan(_cloneButton, 2);
        body.Controls.Add(NewSeparator(), 0, 2);
        body.SetColumnSpan(body.GetControlFromPosition(0, 2)!, 2);
        body.Controls.Add(_tagBox, 0, 3);
        body.Controls.Add(_fetchTagsButton, 1, 3);
        body.Controls.Add(_checkoutButton, 0, 4);
        body.SetColumnSpan(_checkoutButton, 2);
        body.Controls.Add(_checkoutStatus, 0, 5);
        body.SetColumnSpan(_checkoutStatus, 2);
        return panel;
    }

    private Control BuildServices()
    {
        var panel = NewGroup("サービス");
        var body = new TableLayoutPanel
        {
            Dock = DockStyle.Fill,
            RowCount = 2,
            ColumnCount = 1,
        };
        body.RowStyles.Add(new RowStyle(SizeType.AutoSize));
        body.RowStyles.Add(new RowStyle(SizeType.Percent, 100));
        panel.Controls.Add(body);

        var buttons = new FlowLayoutPanel { Dock = DockStyle.Top, AutoSize = true };
        _upButton.Text = "起動";
        _downButton.Text = "停止";
        _refreshServicesButton.Text = "状態更新";
        _upButton.AutoSize = _downButton.AutoSize = _refreshServicesButton.AutoSize = true;
        _serviceUrlLink.AutoSize = true;
        _sslCheckBox.Text = "SSL";
        _sslCheckBox.AutoSize = true;
        _sslCheckBox.Margin = new Padding(8, 8, 4, 4);
        _createSslCertificateButton.Text = "SSL\u8a3c\u660e\u66f8\u4f5c\u6210";
        _createSslCertificateButton.AutoSize = true;
        _downloadCaCertificateButton.Text = "CA\u30c0\u30a6\u30f3\u30ed\u30fc\u30c9";
        _downloadCaCertificateButton.AutoSize = true;
        _serviceUrlLink.Visible = false;
        _serviceUrlLink.Margin = new Padding(16, 8, 4, 4);
        _serviceUrlLink.LinkClicked += (_, _) => OpenServiceUrl();
        _upButton.Click += async (_, _) => await RunBusyAsync(async () =>
        {
            await DockerCompose().UpAsync(AppendLog, _sslCheckBox.Checked);
            await RefreshServicesAsync();
        });
        _downButton.Click += async (_, _) => await RunBusyAsync(async () =>
        {
            await DockerCompose().DownAsync(AppendLog);
            await RefreshServicesAsync();
        });
        _refreshServicesButton.Click += async (_, _) => await RunBusyAsync(RefreshServicesAsync);
        _createSslCertificateButton.Click += async (_, _) => await RunBusyAsync(CreateSslCertificateAsync);
        _downloadCaCertificateButton.Click += async (_, _) => await RunBusyAsync(DownloadCaCertificateAsync);
        buttons.Controls.AddRange(new Control[] { _upButton, _sslCheckBox, _downButton, _refreshServicesButton, _createSslCertificateButton, _downloadCaCertificateButton, _serviceUrlLink });

        _serviceList.Dock = DockStyle.Fill;
        _serviceList.View = View.Details;
        _serviceList.FullRowSelect = true;
        _serviceList.GridLines = true;
        _serviceList.Columns.Add("Service", 180);
        _serviceList.Columns.Add("State", 120);
        _serviceList.Columns.Add("Status", 280);
        _serviceList.Columns.Add("Ports", 420);
        body.Controls.Add(buttons, 0, 0);
        body.Controls.Add(_serviceList, 0, 1);
        return panel;
    }

    private Control BuildLog()
    {
        var panel = NewGroup("ログ");
        _logBox.Dock = DockStyle.Fill;
        _logBox.Multiline = true;
        _logBox.ReadOnly = true;
        _logBox.ScrollBars = ScrollBars.Vertical;
        _logBox.WordWrap = false;
        panel.Controls.Add(_logBox);
        return panel;
    }

    private Control BuildFooter()
    {
        var label = new Label
        {
            AutoSize = true,
            ForeColor = SystemColors.GrayText,
            Text = "想定配置: app\\phantom-manager.exe と app\\phantom-release",
        };
        return label;
    }

    private async Task RefreshAllAsync()
    {
        await RunBusyAsync(async () =>
        {
            await RefreshChecksAsync();
            LoadExistingEnv();
            await FetchTagsAsync(runFetch: false);
            await RefreshServicesAsync();
        });
    }

    private async Task RefreshChecksAsync()
    {
        var dockerDesktopPath = Path.Combine(Environment.GetFolderPath(Environment.SpecialFolder.ProgramFiles), "Docker", "Docker", "Docker Desktop.exe");
        var hasDockerDesktop = File.Exists(dockerDesktopPath);
        var dockerVersion = await CommandRunner.TryRunAsync(DockerCli.WindowsDockerPath, new[] { "--version" }, ReleaseDir);
        var dockerInfo = await CommandRunner.TryRunAsync(DockerCli.WindowsDockerPath, new[] { "info", "--format", "{{.ServerVersion}}" }, ReleaseDir);
        _dockerStatus.Text = hasDockerDesktop
            ? $"○ Docker Desktop for Windows: インストール済み / {(dockerInfo.ExitCode == 0 ? "○ 起動中" : "× 未起動")} ({dockerVersion.Output.Trim()})"
            : "× Docker Desktop for Windows: 未検出";

        var wslStatus = await new WslEnvironment().GetStatusAsync(AppendLog);
        _ubuntuInstalled = wslStatus.UbuntuInstalled;
        var gitVersion = wslStatus.UbuntuInstalled
            ? await WslCommand.TryBashAsync("git --version")
            : new CommandResult(1, "");
        _gitStatus.Text = gitVersion.ExitCode == 0
            ? $"○ Git in {WslEnvironment.UbuntuDistro}: インストール済み ({gitVersion.Output.Trim()})"
            : $"× Git in {WslEnvironment.UbuntuDistro}: 未検出";

        if (!wslStatus.WslInstalled)
        {
            _wslStatus.Text = "× WSL: 未検出";
        }
        else if (!string.IsNullOrWhiteSpace(wslStatus.Error))
        {
            _wslStatus.Text = $"× WSL: distribution 確認失敗 ({wslStatus.Error})";
        }
        else if (wslStatus.UbuntuInstalled)
        {
            var distributions = string.IsNullOrWhiteSpace(wslStatus.Distributions) ? WslEnvironment.UbuntuDistro : wslStatus.Distributions;
            _wslStatus.Text = $"○ WSL {WslEnvironment.UbuntuDistro}: インストール済み ({distributions})";
        }
        else
        {
            var distributions = string.IsNullOrWhiteSpace(wslStatus.Distributions) ? "なし" : wslStatus.Distributions;
            _wslStatus.Text = $"× WSL {WslEnvironment.UbuntuDistro}: 未インストール (現在: {distributions})";
        }

        _repoStatus.Text = ReleaseRepository().IsReady()
            ? $"○ phantom-release: 検出済み ({ReleaseDir})"
            : $"× phantom-release: 未検出 ({ReleaseDir})";
        _cloneButton.Enabled = !ReleaseRepository().DirectoryExists();
    }

    private void LoadExistingEnv()
    {
        // .env SRC_DIR is fixed for the WSL runtime. Keep the Windows mirror target UI unchanged.
    }

    private void EnsureLogDir()
    {
        try
        {
            Directory.CreateDirectory(AppPaths.LogDir);
        }
        catch (Exception ex)
        {
            AppendLog($"ログディレクトリを作成できませんでした: {ex.Message}");
        }
    }

    private async Task EnsureDataDirAsync()
    {
        try
        {
            var directories = new[]
            {
                @"var\data",
                @"var\internet-app-data",
                @"var\extra-data",
                @"var\log\crow\api",
                @"var\log\crow\jobs",
                @"var\log\crow\crawling",
                @"var\log\fox",
                @"var\log\joker",
                @"var\log\mona",
                @"var\log\navi",
                @"var\log\panther",
                @"var\log\skull",
                @"var\log\violet",
            };

            var paths = directories
                .Select(relativePath => $"{WslCommand.PathArg(ReleaseDir)}/{relativePath.Replace('\\', '/')}")
                .ToArray();
            await WslCommand.RunBashAsync($"mkdir -p {string.Join(" ", paths)}", AppendLog);
        }
        catch (Exception ex)
        {
            AppendLog($"データディレクトリを作成できませんでした: {ex.Message}");
        }
    }

    private async Task FetchTagsAsync() => await FetchTagsAsync(runFetch: true);

    private async Task FetchTagsAsync(bool runFetch)
    {
        var repository = ReleaseRepository();
        if (!repository.IsReady())
        {
            return;
        }

        var tags = await repository.GetTagsAsync(runFetch, AppendLog);
        _tagBox.Items.Clear();
        _tagBox.Items.AddRange(tags);
        if (_tagBox.Items.Count > 0)
        {
            _tagBox.SelectedIndex = 0;
        }
    }

    private async Task CheckoutSelectedTagAsync()
    {
        if (_tagBox.SelectedItem is not string tag || string.IsNullOrWhiteSpace(tag))
        {
            MessageBox.Show(this, "チェックアウトするタグを選択してください。", "phantom-manager", MessageBoxButtons.OK, MessageBoxIcon.Information);
            return;
        }

        await ReleaseRepository().CheckoutTagAsync(tag, AppendLog);
        UpdateCheckoutStatus();
        await RefreshServicesAsync();
    }

    private async Task InstallUbuntuAsync()
    {
        var result = MessageBox.Show(
            this,
            $"{WslEnvironment.UbuntuDistro} を WSL にインストールします。Windows の設定や再起動、Ubuntu の初期ユーザー作成が必要になる場合があります。続行しますか?",
            "WSL Ubuntu install",
            MessageBoxButtons.YesNo,
            MessageBoxIcon.Information,
            MessageBoxDefaultButton.Button2);
        if (result != DialogResult.Yes)
        {
            return;
        }

        await RunBusyAsync(async () =>
        {
            await new WslEnvironment().InstallUbuntuAsync(AppendLog);
            await RefreshChecksAsync();
        });
    }

    private async Task InitializeDatabaseAsync()
    {
        var result = MessageBox.Show(
            this,
            "Elasticsearch のインデックスを削除して作り直します。続行しますか？",
            "データベース初期化",
            MessageBoxButtons.YesNo,
            MessageBoxIcon.Warning,
            MessageBoxDefaultButton.Button2);
        if (result != DialogResult.Yes)
        {
            return;
        }

        await RunBusyAsync(async () =>
        {
            await new ElasticsearchInitializer(ReleaseDir).InitializeAsync(AppendLog);
            await RefreshServicesAsync();
        });
    }

    private async Task CreateSslCertificateAsync()
    {
        if (!ReleaseRepository().IsReady())
        {
            throw new DirectoryNotFoundException($"phantom-release 縺瑚ｦ九▽縺九ｊ縺ｾ縺帙ｓ: {ReleaseDir}");
        }

        var ipAddress = NetworkAddressProvider.GetPreferredLocalIPv4Address();
        if (ipAddress is null)
        {
            throw new InvalidOperationException("IP \u30a2\u30c9\u30ec\u30b9\u3092\u53d6\u5f97\u3067\u304d\u307e\u305b\u3093");
        }

        AppendLog($"SSL certificate IP address: {ipAddress}");
        await new NginxSslCertificateGenerator(ReleaseDir).CreateAsync(ipAddress, AppendLog);
        AppendLog("SSL certificate files created: secrets/nginx/ca, secrets/nginx/tls");
    }

    private async Task DownloadCaCertificateAsync()
    {
        if (!ReleaseRepository().IsReady())
        {
            throw new DirectoryNotFoundException($"phantom-release 縺瑚ｦ九▽縺九ｊ縺ｾ縺帙ｓ: {ReleaseDir}");
        }

        using var dialog = new SaveFileDialog
        {
            Title = "CA certificate save",
            FileName = "phantom-local-ca.crt",
            Filter = "Certificate files (*.crt)|*.crt|All files (*.*)|*.*",
            AddExtension = true,
            DefaultExt = "crt",
            OverwritePrompt = true,
        };
        if (dialog.ShowDialog(this) != DialogResult.OK)
        {
            return;
        }

        var caPath = $"{WslCommand.PathArg(ReleaseDir)}/secrets/nginx/ca/phantom-local-ca.crt";
        var certificate = WslCommand.CaptureBashQuiet($"cat {caPath}");
        if (string.IsNullOrWhiteSpace(certificate))
        {
            throw new FileNotFoundException("CA certificate was not found. Create SSL certificates first.", "secrets/nginx/ca/phantom-local-ca.crt");
        }

        await File.WriteAllTextAsync(dialog.FileName, certificate);
        AppendLog($"CA certificate saved: {dialog.FileName}");
    }

    private async Task RefreshServicesAsync()
    {
        _serviceList.Items.Clear();
        _anyServiceRunning = false;
        if (!ReleaseRepository().IsReady())
        {
            UpdateServiceUrlLink();
            return;
        }

        try
        {
            foreach (var service in await DockerCompose().GetServicesAsync(AppendLog))
            {
                var item = new ListViewItem(service.Name);
                item.SubItems.Add(service.State);
                item.SubItems.Add(service.Status);
                item.SubItems.Add(service.Ports);
                _serviceList.Items.Add(item);
                if (service.IsRunning)
                {
                    _anyServiceRunning = true;
                }
            }
            UpdateServiceUrlLink();
        }
        catch (Exception ex)
        {
            AppendLog(ex.Message);
            UpdateServiceUrlLink();
        }
    }

    private async Task SaveEnvAsync()
    {
        if (!ReleaseRepository().IsReady())
        {
            throw new DirectoryNotFoundException($"phantom-release が見つかりません: {ReleaseDir}");
        }

        if (WslCommand.RunBashQuiet($"test -f {WslCommand.PathArg(EnvSamplePath)}") != 0)
        {
            throw new FileNotFoundException("env.sample が見つかりません。", EnvSamplePath);
        }

        var releaseDir = WslCommand.PathArg(ReleaseDir);
        var srcDir = WslCommand.Quote(FixedEnvSrcDir);
        await WslCommand.RunBashAsync(
            $"cd {releaseDir} && cp env.sample .env && " +
            $"if grep -q '^SRC_DIR=' .env; then sed -i 's#^SRC_DIR=.*#SRC_DIR={FixedEnvSrcDir}#' .env; else printf '\\nSRC_DIR=%s\\n' {srcDir} >> .env; fi && " +
            $"mkdir -p {WslCommand.PathArg(ReleaseDir)}/var/internet-app-data",
            AppendLog);
    }

    private async Task CreateMirrorBatchAsync()
    {
        if (string.IsNullOrWhiteSpace(_origDirBox.Text))
        {
            throw new InvalidOperationException("元データディレクトリを選択してください。");
        }
        var origDir = Path.GetFullPath(_origDirBox.Text.Trim());
        if (!Directory.Exists(origDir))
        {
            throw new DirectoryNotFoundException($"元データディレクトリが存在しません: {origDir}");
        }

        var dataDir = await GetMirrorDataDirAsync();
        MirrorBatchWriter.Create(origDir, dataDir);
    }

    private async Task OpenDataDirAsync()
    {
        var dataDir = await GetMirrorDataDirAsync();
        Process.Start(new ProcessStartInfo("explorer.exe", dataDir)
        {
            UseShellExecute = true,
        });
    }

    private async Task<string> GetMirrorDataDirAsync()
    {
        if (!ReleaseRepository().IsReady())
        {
            throw new DirectoryNotFoundException($"phantom-release が見つかりません: {ReleaseDir}");
        }

        var releaseDir = WslCommand.PathArg(ReleaseDir);
        var result = await WslCommand.RunBashAsync(
            $"mkdir -p {releaseDir}/var/internet-app-data && cd {releaseDir} && pwd -P",
            AppendLog);
        var linuxReleaseDir = result.Output
            .Split(new[] { '\r', '\n' }, StringSplitOptions.RemoveEmptyEntries)
            .LastOrDefault();
        if (string.IsNullOrWhiteSpace(linuxReleaseDir))
        {
            throw new InvalidOperationException("WSL 上の phantom-release ディレクトリを解決できませんでした。");
        }

        return ToWslUncPath(WslEnvironment.UbuntuDistro, $"{linuxReleaseDir.TrimEnd('/')}/var/internet-app-data");
    }

    private static string ToWslUncPath(string distribution, string linuxPath)
    {
        var relativePath = linuxPath.Trim().TrimStart('/').Replace('/', '\\');
        return $@"\\wsl.localhost\{distribution}\{relativePath}";
    }

    private GitRepository ReleaseRepository()
    {
        return new GitRepository(ReleaseDir);
    }

    private DockerComposeClient DockerCompose()
    {
        return new DockerComposeClient(ReleaseDir);
    }

    private bool IsTagCheckedOut()
    {
        return ReleaseRepository().GetCheckedOutTag() is not null;
    }

    private bool EnvExists()
    {
        return WslCommand.RunBashQuiet($"test -f {WslCommand.PathArg(EnvPath)}") == 0;
    }

    private void UpdateCheckoutStatus()
    {
        var tag = ReleaseRepository().GetCheckedOutTag();
        _checkoutStatus.Text = tag is null
            ? "チェックアウトされていません。バージョンを選んで「チェックアウトしてください」"
            : $"現在のバージョン: {tag}";
    }

    private async Task RunBusyAsync(Func<Task> action)
    {
        SetBusy(true);
        try
        {
            await action();
        }
        catch (Exception ex)
        {
            AppendLog(ex.Message);
            MessageBox.Show(this, ex.Message, "phantom-manager", MessageBoxButtons.OK, MessageBoxIcon.Error);
        }
        finally
        {
            SetBusy(false);
        }
    }

    private void SetBusy(bool busy)
    {
        Cursor = busy ? Cursors.WaitCursor : Cursors.Default;
        var repoReady = ReleaseRepository().IsReady();
        var canStartCompose = repoReady && EnvExists() && IsTagCheckedOut() && !_anyServiceRunning;
        var canStopOrRefreshServices = repoReady && _anyServiceRunning;
        UpdateCheckoutStatus();
        UpdateServiceUrlLink();
        _cloneButton.Enabled = !busy && !ReleaseRepository().DirectoryExists();
        _refreshChecksButton.Enabled = !busy && !_anyServiceRunning;
        _installUbuntuButton.Enabled = !busy && !_anyServiceRunning && !_ubuntuInstalled;
        _initializeDatabaseButton.Enabled = !busy && repoReady && _anyServiceRunning;
        _saveEnvButton.Enabled = !busy && repoReady && !_anyServiceRunning;
        _selectOrigButton.Enabled = !busy && !_anyServiceRunning;
        _createMirrorBatchButton.Enabled = !busy && !_anyServiceRunning;
        _openDataDirButton.Enabled = !busy && repoReady;
        _upButton.Enabled = !busy && canStartCompose;
        _sslCheckBox.Enabled = !busy && !_anyServiceRunning;
        _downButton.Enabled = !busy && canStopOrRefreshServices;
        _refreshServicesButton.Enabled = !busy && canStopOrRefreshServices;
        _createSslCertificateButton.Enabled = !busy && repoReady && !_anyServiceRunning;
        _downloadCaCertificateButton.Enabled = !busy && repoReady;
        _fetchTagsButton.Enabled = !busy && repoReady && !_anyServiceRunning;
        _checkoutButton.Enabled = !busy && repoReady && !_anyServiceRunning;
    }

    private void AppendLog(string message)
    {
        if (InvokeRequired)
        {
            BeginInvoke(() => AppendLog(message));
            return;
        }
        _logBox.AppendText($"[{DateTime.Now:HH:mm:ss}] {message}{Environment.NewLine}");
    }

    private void UpdateServiceUrlLink()
    {
        if (!_anyServiceRunning)
        {
            _serviceUrlLink.Visible = false;
            _serviceUrlLink.Text = "";
            return;
        }

        var ipAddress = NetworkAddressProvider.GetPreferredLocalIPv4Address();
        if (ipAddress is null)
        {
            _serviceUrlLink.Visible = true;
            _serviceUrlLink.Text = "IP アドレスを取得できません";
            _serviceUrlLink.Links.Clear();
            return;
        }

        var protocol = _sslCheckBox.Checked ? "https" : "http";
        var url = $"{protocol}://{ipAddress}:8080/";
        _serviceUrlLink.Visible = true;
        _serviceUrlLink.Text = url;
        _serviceUrlLink.Links.Clear();
        _serviceUrlLink.Links.Add(0, url.Length, url);
    }

    private void OpenServiceUrl()
    {
        if (_serviceUrlLink.Links.Count == 0 || _serviceUrlLink.Links[0].LinkData is not string url)
        {
            return;
        }

        try
        {
            Process.Start(new ProcessStartInfo(url) { UseShellExecute = true });
        }
        catch (Exception ex)
        {
            AppendLog($"リンクを開けませんでした: {ex.Message}");
            MessageBox.Show(this, ex.Message, "phantom-manager", MessageBoxButtons.OK, MessageBoxIcon.Error);
        }
    }

    private static Button NewButton(string text) => new()
    {
        Text = text,
        AutoSize = true,
        Margin = new Padding(4),
    };

    private static Panel NewSpacer(int height) => new()
    {
        Height = height,
        Dock = DockStyle.Top,
        Margin = new Padding(0),
    };

    private static Panel NewSeparator() => new()
    {
        Height = 18,
        Dock = DockStyle.Top,
        Margin = new Padding(0, 8, 0, 8),
        BorderStyle = BorderStyle.Fixed3D,
    };

    private static GroupBox NewGroup(string text) => new()
    {
        Text = text,
        Dock = DockStyle.Fill,
        Padding = new Padding(10),
        Margin = new Padding(0, 0, 10, 10),
        MinimumSize = new Size(0, 310),
    };

    private static FlowLayoutPanel NewVertical() => new()
    {
        Dock = DockStyle.Fill,
        FlowDirection = FlowDirection.TopDown,
        WrapContents = false,
        AutoScroll = true,
    };

    private static void ConfigureStatusLabel(Label label)
    {
        label.AutoSize = true;
        label.MaximumSize = new Size(360, 0);
        label.Margin = new Padding(0, 2, 0, 2);
    }
}
