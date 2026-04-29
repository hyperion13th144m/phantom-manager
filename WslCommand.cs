namespace PhantomManager;

internal static class WslCommand
{
    public static Task<CommandResult> RunBashAsync(string script, Action<string>? log = null)
    {
        return CommandRunner.RunAsync("wsl.exe", BashArgs(script), AppContext.BaseDirectory, log);
    }

    public static Task<CommandResult> TryBashAsync(string script, Action<string>? log = null)
    {
        return CommandRunner.TryRunAsync("wsl.exe", BashArgs(script), AppContext.BaseDirectory, log);
    }

    public static int RunBashQuiet(string script)
    {
        return CommandRunner.RunQuiet("wsl.exe", BashArgs(script), AppContext.BaseDirectory);
    }

    public static string? CaptureBashQuiet(string script)
    {
        return CommandRunner.CaptureQuiet("wsl.exe", BashArgs(script), AppContext.BaseDirectory);
    }

    public static string PathArg(string path)
    {
        var normalized = path.Trim().Replace('\\', '/');
        if (normalized == "~")
        {
            return "$HOME";
        }

        if (normalized.StartsWith("~/", StringComparison.Ordinal))
        {
            return "$HOME/" + Quote(normalized[2..]);
        }

        return Quote(normalized);
    }

    public static string ParentPathArg(string path)
    {
        var normalized = path.Trim().Replace('\\', '/').TrimEnd('/');
        var slashIndex = normalized.LastIndexOf('/');
        if (slashIndex <= 0)
        {
            return ".";
        }

        return PathArg(normalized[..slashIndex]);
    }

    public static string Quote(string value)
    {
        return "'" + value.Replace("'", "'\"'\"'") + "'";
    }

    private static string[] BashArgs(string script)
    {
        return new[] { "-d", WslEnvironment.UbuntuDistro, "--", "bash", "-lc", script };
    }
}
