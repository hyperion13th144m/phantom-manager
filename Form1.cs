using System.Diagnostics;
using System.Text;
using System.Text.RegularExpressions;

namespace PhantomManager;

public partial class Form1 : Form
{
    private readonly TextBox _releasePathBox = new();
    private readonly TextBox _srcDirBox = new();
    private readonly TextBox _origDirBox = new();
    private readonly TextBox _logBox = new();
    private readonly ComboBox _tagBox = new();
    private readonly ListView _serviceList = new();
    private readonly Label _dockerStatus = new();
    private readonly Label _gitStatus = new();
    private readonly Label _repoStatus = new();
    private readonly Label _checkoutStatus = new();
    private readonly LinkLabel _serviceUrlLink = new();
    private readonly Button _upButton = new();
    private readonly Button _downButton = new();
    private readonly Button _refreshServicesButton = new();
    private readonly Button _fetchTagsButton = new();
    private readonly Button _checkoutButton = new();
    private readonly Button _saveEnvButton = new();
    private readonly Button _selectOrigButton = new();
    private readonly Button _createMirrorBatchButton = new();
    private readonly Button _refreshChecksButton = new();
    private readonly Button _initializeDatabaseButton = new();
    private readonly Button _cloneButton = new();
    private bool _anyServiceRunning;

    public Form1()
    {
        InitializeComponent();
        BuildUi();
        EnsureDefaultSrcDir();
        EnsureLogDir();
        Shown += async (_, _) => await RefreshAllAsync();
    }

    private string ReleaseDir => _releasePathBox.Text.Trim();
    private string EnvPath => Path.Combine(ReleaseDir, ".env");
    private string EnvSamplePath => Path.Combine(ReleaseDir, "env.sample");

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
            ColumnCount = 4,
            Padding = new Padding(0, 0, 0, 10),
        };
        panel.ColumnStyles.Add(new ColumnStyle(SizeType.AutoSize));
        panel.ColumnStyles.Add(new ColumnStyle(SizeType.Percent, 100));
        panel.ColumnStyles.Add(new ColumnStyle(SizeType.AutoSize));
        panel.ColumnStyles.Add(new ColumnStyle(SizeType.AutoSize));

        var title = new Label
        {
            Text = "phantom 全文検索システム管理",
            Font = new Font(Font.FontFamily, 14F, FontStyle.Bold),
            AutoSize = true,
            Padding = new Padding(0, 0, 24, 0),
        };

        _releasePathBox.Text = AppPaths.DefaultReleaseDir;
        _releasePathBox.Anchor = AnchorStyles.Left | AnchorStyles.Right;

        var browseButton = NewButton("参照");
        browseButton.Click += (_, _) =>
        {
            using var dialog = new FolderBrowserDialog
            {
                Description = "phantom-release ディレクトリを選択してください",
                SelectedPath = Directory.Exists(ReleaseDir) ? ReleaseDir : AppContext.BaseDirectory,
                UseDescriptionForTitle = true,
            };
            if (dialog.ShowDialog(this) == DialogResult.OK)
            {
                _releasePathBox.Text = dialog.SelectedPath;
                _ = RefreshAllAsync();
            }
        };

        _cloneButton.Text = "clone";
        _cloneButton.AutoSize = true;
        _cloneButton.Click += async (_, _) => await RunBusyAsync(async () =>
        {
            await CommandRunner.RunAsync("git", new[] { "clone", "https://github.com/hyperion13th144m/phantom-release", ReleaseDir }, AppContext.BaseDirectory, AppendLog);
            await RefreshAllAsync();
        });

        panel.Controls.Add(title, 0, 0);
        panel.Controls.Add(_releasePathBox, 1, 0);
        panel.Controls.Add(browseButton, 2, 0);
        panel.Controls.Add(_cloneButton, 3, 0);
        return panel;
    }

    private Control BuildActions()
    {
        var grid = new TableLayoutPanel
        {
            Dock = DockStyle.Top,
            AutoSize = false,
            Height = 330,
            MinimumSize = new Size(0, 330),
            ColumnCount = 3,
            Padding = new Padding(0, 0, 0, 10),
        };
        grid.ColumnStyles.Add(new ColumnStyle(SizeType.Percent, 34));
        grid.ColumnStyles.Add(new ColumnStyle(SizeType.Percent, 33));
        grid.ColumnStyles.Add(new ColumnStyle(SizeType.Percent, 33));

        grid.Controls.Add(BuildStatusPanel(), 0, 0);
        grid.Controls.Add(BuildEnvironmentPanel(), 1, 0);
        grid.Controls.Add(BuildVersionPanel(), 2, 0);
        return grid;
    }

    private Control BuildStatusPanel()
    {
        var panel = NewGroup("環境チェック");
        var body = NewVertical();
        panel.Controls.Add(body);

        ConfigureStatusLabel(_dockerStatus);
        ConfigureStatusLabel(_gitStatus);
        ConfigureStatusLabel(_repoStatus);
        _dockerStatus.Text = "Docker Desktop for Windows: 確認中";
        _gitStatus.Text = "Git for Windows: 確認中";
        _repoStatus.Text = "phantom-release: 確認中";
        body.Controls.Add(_dockerStatus);
        body.Controls.Add(_gitStatus);
        body.Controls.Add(_repoStatus);

        _refreshChecksButton.Text = "再チェック";
        _refreshChecksButton.AutoSize = true;
        _refreshChecksButton.Click += async (_, _) => await RefreshAllAsync();
        body.Controls.Add(_refreshChecksButton);

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
            RowCount = 4,
            ColumnCount = 2,
        };
        body.RowStyles.Add(new RowStyle(SizeType.AutoSize));
        body.RowStyles.Add(new RowStyle(SizeType.AutoSize));
        body.RowStyles.Add(new RowStyle(SizeType.AutoSize));
        body.RowStyles.Add(new RowStyle(SizeType.AutoSize));
        body.ColumnStyles.Add(new ColumnStyle(SizeType.Percent, 100));
        body.ColumnStyles.Add(new ColumnStyle(SizeType.AutoSize));
        panel.Controls.Add(body);

        _srcDirBox.Anchor = AnchorStyles.Left | AnchorStyles.Right;
        _srcDirBox.Text = AppPaths.DefaultSrcDir;
        var chooseButton = NewButton("選択");
        chooseButton.Click += (_, _) =>
        {
            using var dialog = new FolderBrowserDialog
            {
                Description = "インターネット出願ソフト等からコピーしたデータの保存ディレクトリを選択してください",
                SelectedPath = Directory.Exists(_srcDirBox.Text) ? _srcDirBox.Text : Environment.GetFolderPath(Environment.SpecialFolder.MyDocuments),
                UseDescriptionForTitle = true,
            };
            if (dialog.ShowDialog(this) == DialogResult.OK)
            {
                _srcDirBox.Text = dialog.SelectedPath;
            }
        };

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
            SaveEnv();
            AppendLog($".env を保存しました: {EnvPath}");
            await Task.CompletedTask;
        });

        _createMirrorBatchButton.Text = "ミラーバッチ作成";
        _createMirrorBatchButton.AutoSize = true;
        _createMirrorBatchButton.Click += async (_, _) => await RunBusyAsync(async () =>
        {
            CreateMirrorBatch();
            AppendLog($"ミラーバッチを作成しました: {AppPaths.MirrorBatPath}");
            await Task.CompletedTask;
        });

        var actionButtons = new FlowLayoutPanel
        {
            Dock = DockStyle.Top,
            AutoSize = true,
            FlowDirection = FlowDirection.LeftToRight,
            WrapContents = true,
            Margin = new Padding(0),
        };
        actionButtons.Controls.AddRange(new Control[] { _saveEnvButton, _createMirrorBatchButton });

        body.Controls.Add(_srcDirBox, 0, 0);
        body.Controls.Add(chooseButton, 1, 0);
        body.Controls.Add(_origDirBox, 0, 1);
        body.Controls.Add(_selectOrigButton, 1, 1);
        body.Controls.Add(actionButtons, 0, 2);
        body.SetColumnSpan(actionButtons, 2);
        return panel;
    }

    private Control BuildVersionPanel()
    {
        var panel = NewGroup("バージョン");
        var body = new TableLayoutPanel
        {
            Dock = DockStyle.Top,
            AutoSize = true,
            RowCount = 3,
            ColumnCount = 2,
        };
        body.RowStyles.Add(new RowStyle(SizeType.AutoSize));
        body.RowStyles.Add(new RowStyle(SizeType.AutoSize));
        body.RowStyles.Add(new RowStyle(SizeType.AutoSize));
        body.ColumnStyles.Add(new ColumnStyle(SizeType.Percent, 100));
        body.ColumnStyles.Add(new ColumnStyle(SizeType.AutoSize));
        panel.Controls.Add(body);

        _tagBox.DropDownStyle = ComboBoxStyle.DropDownList;
        _tagBox.Anchor = AnchorStyles.Left | AnchorStyles.Right;
        ConfigureStatusLabel(_checkoutStatus);
        _checkoutStatus.MaximumSize = new Size(420, 0);
        _checkoutStatus.Text = "チェックアウト状態: 確認中";

        _fetchTagsButton.Text = "fetch / タグ取得";
        _fetchTagsButton.AutoSize = true;
        _fetchTagsButton.Click += async (_, _) => await RunBusyAsync(FetchTagsAsync);

        _checkoutButton.Text = "チェックアウト";
        _checkoutButton.AutoSize = true;
        _checkoutButton.Click += async (_, _) => await RunBusyAsync(CheckoutSelectedTagAsync);

        body.Controls.Add(_tagBox, 0, 0);
        body.Controls.Add(_fetchTagsButton, 1, 0);
        body.Controls.Add(_checkoutButton, 0, 1);
        body.SetColumnSpan(_checkoutButton, 2);
        body.Controls.Add(_checkoutStatus, 0, 2);
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
        _serviceUrlLink.Visible = false;
        _serviceUrlLink.Margin = new Padding(16, 8, 4, 4);
        _serviceUrlLink.LinkClicked += (_, _) => OpenServiceUrl();
        _upButton.Click += async (_, _) => await RunBusyAsync(async () =>
        {
            await DockerCompose().UpAsync(AppendLog);
            await RefreshServicesAsync();
        });
        _downButton.Click += async (_, _) => await RunBusyAsync(async () =>
        {
            await DockerCompose().DownAsync(AppendLog);
            await RefreshServicesAsync();
        });
        _refreshServicesButton.Click += async (_, _) => await RunBusyAsync(RefreshServicesAsync);
        buttons.Controls.AddRange(new Control[] { _upButton, _downButton, _refreshServicesButton, _serviceUrlLink });

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
        var dockerVersion = await CommandRunner.TryRunAsync("docker", new[] { "--version" }, ReleaseDir);
        var dockerInfo = await CommandRunner.TryRunAsync("docker", new[] { "info", "--format", "{{.ServerVersion}}" }, ReleaseDir);
        _dockerStatus.Text = hasDockerDesktop
            ? $"○ Docker Desktop for Windows: インストール済み / {(dockerInfo.ExitCode == 0 ? "起動中" : "未起動")} ({dockerVersion.Output.Trim()})"
            : "× Docker Desktop for Windows: 未検出";

        var gitVersion = await CommandRunner.TryRunAsync("git", new[] { "--version" }, ReleaseDir);
        _gitStatus.Text = gitVersion.ExitCode == 0
            ? $"○ Git for Windows: インストール済み ({gitVersion.Output.Trim()})"
            : "× Git for Windows: 未検出";

        _repoStatus.Text = ReleaseRepository().IsReady()
            ? $"○ phantom-release: 検出済み ({ReleaseDir})"
            : $"× phantom-release: 未検出 ({ReleaseDir})";
        _cloneButton.Enabled = !Directory.Exists(ReleaseDir);
    }

    private void LoadExistingEnv()
    {
        if (!File.Exists(EnvPath))
        {
            return;
        }

        var text = File.ReadAllText(EnvPath, Encoding.UTF8);
        var match = Regex.Match(text, @"(?m)^SRC_DIR=(.*)$");
        if (match.Success)
        {
            _srcDirBox.Text = match.Groups[1].Value.Trim().Trim('"').Replace('/', Path.DirectorySeparatorChar);
        }
    }

    private void EnsureDefaultSrcDir()
    {
        try
        {
            Directory.CreateDirectory(AppPaths.DefaultSrcDir);
        }
        catch (Exception ex)
        {
            AppendLog($"デフォルトのデータディレクトリを作成できませんでした: {ex.Message}");
        }
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

    private void SaveEnv()
    {
        if (!Directory.Exists(ReleaseDir))
        {
            throw new DirectoryNotFoundException($"phantom-release が見つかりません: {ReleaseDir}");
        }
        if (!File.Exists(EnvSamplePath))
        {
            throw new FileNotFoundException("env.sample が見つかりません。", EnvSamplePath);
        }
        if (string.IsNullOrWhiteSpace(_srcDirBox.Text))
        {
            throw new InvalidOperationException("データディレクトリを選択してください。");
        }

        var srcDirPath = Path.GetFullPath(_srcDirBox.Text.Trim());
        if (!Directory.Exists(srcDirPath))
        {
            throw new DirectoryNotFoundException($"指定したデータディレクトリが存在しません: {srcDirPath}");
        }

        var srcDir = srcDirPath.Replace('\\', '/');
        var text = File.ReadAllText(EnvSamplePath, Encoding.UTF8);
        if (Regex.IsMatch(text, @"(?m)^SRC_DIR=.*$"))
        {
            text = Regex.Replace(text, @"(?m)^SRC_DIR=.*$", $"SRC_DIR={srcDir}");
        }
        else
        {
            text = $"SRC_DIR={srcDir}{Environment.NewLine}{text}";
        }
        File.WriteAllText(EnvPath, text, new UTF8Encoding(false));
    }

    private void CreateMirrorBatch()
    {
        if (string.IsNullOrWhiteSpace(_origDirBox.Text))
        {
            throw new InvalidOperationException("元データディレクトリを選択してください。");
        }
        if (string.IsNullOrWhiteSpace(_srcDirBox.Text))
        {
            throw new InvalidOperationException("データディレクトリを選択してください。");
        }

        var origDir = Path.GetFullPath(_origDirBox.Text.Trim());
        var dataDir = Path.GetFullPath(_srcDirBox.Text.Trim());
        if (!Directory.Exists(origDir))
        {
            throw new DirectoryNotFoundException($"元データディレクトリが存在しません: {origDir}");
        }
        if (!Directory.Exists(dataDir))
        {
            throw new DirectoryNotFoundException($"データディレクトリが存在しません: {dataDir}");
        }

        MirrorBatchWriter.Create(origDir, dataDir);
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
        var canStartCompose = repoReady && File.Exists(EnvPath) && IsTagCheckedOut() && !_anyServiceRunning;
        var canStopOrRefreshServices = repoReady && _anyServiceRunning;
        UpdateCheckoutStatus();
        UpdateServiceUrlLink();
        _cloneButton.Enabled = !busy && !Directory.Exists(ReleaseDir);
        _refreshChecksButton.Enabled = !busy && !_anyServiceRunning;
        _initializeDatabaseButton.Enabled = !busy && repoReady && _anyServiceRunning;
        _saveEnvButton.Enabled = !busy && Directory.Exists(ReleaseDir) && !_anyServiceRunning;
        _selectOrigButton.Enabled = !busy && !_anyServiceRunning;
        _createMirrorBatchButton.Enabled = !busy && !_anyServiceRunning;
        _upButton.Enabled = !busy && canStartCompose;
        _downButton.Enabled = !busy && canStopOrRefreshServices;
        _refreshServicesButton.Enabled = !busy && canStopOrRefreshServices;
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

        var url = $"http://{ipAddress}:8080/";
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
