namespace PhantomManager;

internal sealed class WslEnvironment
{
    public const string UbuntuDistro = "Ubuntu-20.04";

    public async Task<WslStatus> GetStatusAsync(Action<string>? log = null)
    {
        var wslVersion = await CommandRunner.TryRunAsync("wsl.exe", new[] { "--version" }, AppContext.BaseDirectory);
        var wslInstalled = wslVersion.ExitCode == 0;
        if (!wslInstalled)
        {
            var legacyStatus = await CommandRunner.TryRunAsync("wsl.exe", new[] { "--status" }, AppContext.BaseDirectory);
            wslInstalled = legacyStatus.ExitCode == 0;
        }

        if (!wslInstalled)
        {
            return new WslStatus(false, false, "", "wsl.exe is not available.");
        }

        var distributions = await CommandRunner.TryRunAsync("wsl.exe", new[] { "--list", "--quiet" }, AppContext.BaseDirectory, log);
        if (distributions.ExitCode != 0)
        {
            return new WslStatus(true, false, "", Normalize(distributions.Output));
        }

        var names = Normalize(distributions.Output)
            .Split(new[] { '\r', '\n' }, StringSplitOptions.RemoveEmptyEntries)
            .Select(line => line.Trim().TrimEnd('*').Trim())
            .Where(line => !string.IsNullOrWhiteSpace(line))
            .ToArray();

        var ubuntuInstalled = names.Any(name => string.Equals(name, UbuntuDistro, StringComparison.OrdinalIgnoreCase));
        return new WslStatus(true, ubuntuInstalled, string.Join(", ", names), "");
    }

    public async Task InstallUbuntuAsync(Action<string>? log)
    {
        await CommandRunner.RunAsync("wsl.exe", new[] { "--install", "-d", UbuntuDistro }, AppContext.BaseDirectory, log);
    }

    private static string Normalize(string text)
    {
        return text.Replace("\0", "").Trim();
    }
}

internal sealed record WslStatus(bool WslInstalled, bool UbuntuInstalled, string Distributions, string Error);
