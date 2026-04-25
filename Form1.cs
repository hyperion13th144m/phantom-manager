using System.ComponentModel;
using System.Diagnostics;
using System.Net;
using System.Net.NetworkInformation;
using System.Net.Sockets;
using System.Text;
using System.Text.Json;
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
    private static string DefaultSrcDir => Path.Combine(AppContext.BaseDirectory, "インターネット出願ソフトのデータ");
    private static string LogDir => Path.Combine(AppContext.BaseDirectory, "log");
    private static string BatDir => Path.Combine(AppContext.BaseDirectory, "bat");
    private static string MirrorBatPath => Path.Combine(BatDir, "mirror.bat");

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

        _releasePathBox.Text = Path.Combine(AppContext.BaseDirectory, "phantom-release");
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
            await RunCommandAsync("git", new[] { "clone", "https://github.com/hyperion13th144m/phantom-release", ReleaseDir }, AppContext.BaseDirectory);
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
        _srcDirBox.Text = DefaultSrcDir;
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
            AppendLog($"ミラーバッチを作成しました: {MirrorBatPath}");
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
            await RunCommandAsync("docker", new[] { "compose", "up", "-d" }, ReleaseDir);
            await RefreshServicesAsync();
        });
        _downButton.Click += async (_, _) => await RunBusyAsync(async () =>
        {
            await RunCommandAsync("docker", new[] { "compose", "down" }, ReleaseDir);
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
        var dockerVersion = await TryRunCommandAsync("docker", new[] { "--version" }, ReleaseDir, log: false);
        var dockerInfo = await TryRunCommandAsync("docker", new[] { "info", "--format", "{{.ServerVersion}}" }, ReleaseDir, log: false);
        _dockerStatus.Text = hasDockerDesktop
            ? $"○ Docker Desktop for Windows: インストール済み / {(dockerInfo.ExitCode == 0 ? "起動中" : "未起動")} ({dockerVersion.Output.Trim()})"
            : "× Docker Desktop for Windows: 未検出";

        var gitVersion = await TryRunCommandAsync("git", new[] { "--version" }, ReleaseDir, log: false);
        _gitStatus.Text = gitVersion.ExitCode == 0
            ? $"○ Git for Windows: インストール済み ({gitVersion.Output.Trim()})"
            : "× Git for Windows: 未検出";

        _repoStatus.Text = IsReleaseRepoReady()
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
            Directory.CreateDirectory(DefaultSrcDir);
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
            Directory.CreateDirectory(LogDir);
        }
        catch (Exception ex)
        {
            AppendLog($"ログディレクトリを作成できませんでした: {ex.Message}");
        }
    }

    private async Task FetchTagsAsync() => await FetchTagsAsync(runFetch: true);

    private async Task FetchTagsAsync(bool runFetch)
    {
        if (!IsReleaseRepoReady())
        {
            return;
        }

        if (runFetch)
        {
            await RunCommandAsync("git", new[] { "fetch", "--tags", "--prune" }, ReleaseDir);
        }

        var result = await RunCommandAsync("git", new[] { "tag", "--list", "--sort=-v:refname" }, ReleaseDir);
        var tags = result.Output.Split(new[] { '\r', '\n' }, StringSplitOptions.RemoveEmptyEntries);
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

        await RunCommandAsync("git", new[] { "checkout", tag }, ReleaseDir);
        UpdateCheckoutStatus();
        await RefreshServicesAsync();
    }

    private async Task RefreshServicesAsync()
    {
        _serviceList.Items.Clear();
        _anyServiceRunning = false;
        if (!IsReleaseRepoReady())
        {
            UpdateServiceUrlLink();
            return;
        }

        var result = await TryRunCommandAsync("docker", new[] { "compose", "ps", "--all", "--format", "json" }, ReleaseDir);
        if (result.ExitCode != 0)
        {
            UpdateServiceUrlLink();
            return;
        }

        try
        {
            foreach (var line in result.Output.Split(new[] { '\r', '\n' }, StringSplitOptions.RemoveEmptyEntries))
            {
                using var document = JsonDocument.Parse(line);
                var service = document.RootElement;
                var item = new ListViewItem(GetJsonString(service, "Service"));
                var state = GetJsonString(service, "State");
                item.SubItems.Add(state);
                item.SubItems.Add(GetJsonString(service, "Status"));
                item.SubItems.Add(FormatPorts(service));
                _serviceList.Items.Add(item);
                if (IsRunningState(state))
                {
                    _anyServiceRunning = true;
                }
            }
            UpdateServiceUrlLink();
        }
        catch (JsonException)
        {
            foreach (var line in result.Output.Split(new[] { '\r', '\n' }, StringSplitOptions.RemoveEmptyEntries))
            {
                _serviceList.Items.Add(new ListViewItem(new[] { line, "", "", "" }));
            }
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

        EnsureLogDir();
        Directory.CreateDirectory(BatDir);
        var mirrorLogPath = Path.Combine(LogDir, "mirror.log");
        var batch = string.Join(Environment.NewLine, new[]
        {
            "@echo off",
            "chcp 65001 >nul",
            $"set \"ORIG={origDir}\"",
            $"set \"DATA_DIR={dataDir}\"",
            $"robocopy \"%ORIG%\" \"%DATA_DIR%\" /MIR /S /M /R:3 /W:10 /NP /NDL /LOG:\"{mirrorLogPath}\"",
            "exit /b %ERRORLEVEL%",
            "",
        });
        File.WriteAllText(MirrorBatPath, batch, new UTF8Encoding(encoderShouldEmitUTF8Identifier: true));
    }

    private bool IsReleaseRepoReady()
    {
        return Directory.Exists(ReleaseDir)
            && Directory.Exists(Path.Combine(ReleaseDir, ".git"))
            && File.Exists(Path.Combine(ReleaseDir, "docker-compose.yml"));
    }

    private bool IsTagCheckedOut()
    {
        return GetCheckedOutTag() is not null;
    }

    private string? GetCheckedOutTag()
    {
        if (!IsReleaseRepoReady())
        {
            return null;
        }

        var isDetached = RunGitQuiet(new[] { "symbolic-ref", "-q", "HEAD" }) != 0;
        if (!isDetached)
        {
            return null;
        }

        var tag = RunGitCapture(new[] { "describe", "--exact-match", "--tags", "HEAD" });
        return string.IsNullOrWhiteSpace(tag) ? null : tag.Trim();
    }

    private int RunGitQuiet(IReadOnlyList<string> args)
    {
        try
        {
            var startInfo = new ProcessStartInfo("git")
            {
                WorkingDirectory = ReleaseDir,
                UseShellExecute = false,
                RedirectStandardOutput = true,
                RedirectStandardError = true,
                CreateNoWindow = true,
                StandardOutputEncoding = Encoding.UTF8,
                StandardErrorEncoding = Encoding.UTF8,
            };
            foreach (var arg in args)
            {
                startInfo.ArgumentList.Add(arg);
            }

            using var process = Process.Start(startInfo);
            if (process is null)
            {
                return -1;
            }
            if (!process.WaitForExit(3000))
            {
                process.Kill();
                return -1;
            }
            return process.ExitCode;
        }
        catch
        {
            return -1;
        }
    }

    private string? RunGitCapture(IReadOnlyList<string> args)
    {
        try
        {
            var startInfo = new ProcessStartInfo("git")
            {
                WorkingDirectory = ReleaseDir,
                UseShellExecute = false,
                RedirectStandardOutput = true,
                RedirectStandardError = true,
                CreateNoWindow = true,
                StandardOutputEncoding = Encoding.UTF8,
                StandardErrorEncoding = Encoding.UTF8,
            };
            foreach (var arg in args)
            {
                startInfo.ArgumentList.Add(arg);
            }

            using var process = Process.Start(startInfo);
            if (process is null)
            {
                return null;
            }
            var output = process.StandardOutput.ReadToEnd();
            process.WaitForExit(3000);
            return process.ExitCode == 0 ? output : null;
        }
        catch
        {
            return null;
        }
    }

    private void UpdateCheckoutStatus()
    {
        var tag = GetCheckedOutTag();
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
        var repoReady = IsReleaseRepoReady();
        var canStartCompose = repoReady && File.Exists(EnvPath) && IsTagCheckedOut() && !_anyServiceRunning;
        var canStopOrRefreshServices = repoReady && _anyServiceRunning;
        UpdateCheckoutStatus();
        UpdateServiceUrlLink();
        _cloneButton.Enabled = !busy && !Directory.Exists(ReleaseDir);
        _refreshChecksButton.Enabled = !busy && !_anyServiceRunning;
        _saveEnvButton.Enabled = !busy && Directory.Exists(ReleaseDir) && !_anyServiceRunning;
        _selectOrigButton.Enabled = !busy && !_anyServiceRunning;
        _createMirrorBatchButton.Enabled = !busy && !_anyServiceRunning;
        _upButton.Enabled = !busy && canStartCompose;
        _downButton.Enabled = !busy && canStopOrRefreshServices;
        _refreshServicesButton.Enabled = !busy && canStopOrRefreshServices;
        _fetchTagsButton.Enabled = !busy && repoReady && !_anyServiceRunning;
        _checkoutButton.Enabled = !busy && repoReady && !_anyServiceRunning;
    }

    private async Task<CommandResult> RunCommandAsync(string fileName, IReadOnlyList<string> args, string workingDirectory)
    {
        var result = await TryRunCommandAsync(fileName, args, workingDirectory);
        if (result.ExitCode != 0)
        {
            throw new InvalidOperationException($"{fileName} {string.Join(" ", args)} failed: exit code {result.ExitCode}");
        }
        return result;
    }

    private async Task<CommandResult> TryRunCommandAsync(string fileName, IReadOnlyList<string> args, string workingDirectory, bool log = true)
    {
        var output = new StringBuilder();
        var psi = new ProcessStartInfo(fileName)
        {
            WorkingDirectory = Directory.Exists(workingDirectory) ? workingDirectory : AppContext.BaseDirectory,
            UseShellExecute = false,
            RedirectStandardOutput = true,
            RedirectStandardError = true,
            CreateNoWindow = true,
            StandardOutputEncoding = Encoding.UTF8,
            StandardErrorEncoding = Encoding.UTF8,
        };
        foreach (var arg in args)
        {
            psi.ArgumentList.Add(arg);
        }

        if (log)
        {
            AppendLog($"> {fileName} {string.Join(" ", args)}");
        }

        try
        {
            using var process = new Process { StartInfo = psi, EnableRaisingEvents = true };
            process.OutputDataReceived += (_, e) =>
            {
                if (e.Data is not null)
                {
                    output.AppendLine(e.Data);
                    if (log)
                    {
                        BeginInvoke(() => AppendLog(e.Data));
                    }
                }
            };
            process.ErrorDataReceived += (_, e) =>
            {
                if (e.Data is not null)
                {
                    output.AppendLine(e.Data);
                    if (log)
                    {
                        BeginInvoke(() => AppendLog(e.Data));
                    }
                }
            };
            process.Start();
            process.BeginOutputReadLine();
            process.BeginErrorReadLine();
            await process.WaitForExitAsync();
            return new CommandResult(process.ExitCode, output.ToString());
        }
        catch (Win32Exception ex)
        {
            if (log)
            {
                AppendLog(ex.Message);
            }
            return new CommandResult(9009, ex.Message);
        }
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

        var ipAddress = GetPreferredLocalIPv4Address();
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

    private static string? GetPreferredLocalIPv4Address()
    {
        var interfaces = NetworkInterface.GetAllNetworkInterfaces()
            .Where(networkInterface =>
                networkInterface.OperationalStatus == OperationalStatus.Up
                && !networkInterface.Name.Contains("vEthernet", StringComparison.OrdinalIgnoreCase)
                && !networkInterface.Description.Contains("Hyper-V", StringComparison.OrdinalIgnoreCase)
                && (networkInterface.NetworkInterfaceType == NetworkInterfaceType.Ethernet
                    || networkInterface.NetworkInterfaceType == NetworkInterfaceType.Wireless80211))
            .OrderBy(networkInterface => networkInterface.NetworkInterfaceType == NetworkInterfaceType.Ethernet ? 0 : 1);

        foreach (var networkInterface in interfaces)
        {
            var address = networkInterface.GetIPProperties().UnicastAddresses
                .FirstOrDefault(unicast =>
                    unicast.Address.AddressFamily == AddressFamily.InterNetwork
                    && !IPAddress.IsLoopback(unicast.Address)
                    && !unicast.Address.ToString().StartsWith("169.254.", StringComparison.Ordinal));
            if (address is not null)
            {
                return address.Address.ToString();
            }
        }

        return null;
    }

    private static string GetJsonString(JsonElement element, string name)
    {
        return element.TryGetProperty(name, out var value) ? value.ToString() : "";
    }

    private static bool IsRunningState(string state)
    {
        return string.Equals(state, "running", StringComparison.OrdinalIgnoreCase);
    }

    private static string FormatPorts(JsonElement service)
    {
        var ports = GetJsonString(service, "Ports");
        return string.IsNullOrWhiteSpace(ports) ? FormatPublishers(service) : ports;
    }

    private static string FormatPublishers(JsonElement service)
    {
        if (!service.TryGetProperty("Publishers", out var publishers) || publishers.ValueKind != JsonValueKind.Array)
        {
            return "";
        }

        var parts = new List<string>();
        foreach (var publisher in publishers.EnumerateArray())
        {
            var target = GetJsonString(publisher, "TargetPort");
            var published = GetJsonString(publisher, "PublishedPort");
            var protocol = GetJsonString(publisher, "Protocol");
            if (!string.IsNullOrWhiteSpace(published))
            {
                parts.Add($"{published}->{target}/{protocol}");
            }
            else if (!string.IsNullOrWhiteSpace(target))
            {
                parts.Add($"{target}/{protocol}");
            }
        }
        return string.Join(", ", parts);
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

    private sealed record CommandResult(int ExitCode, string Output);
}
