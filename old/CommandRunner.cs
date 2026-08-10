using System.ComponentModel;
using System.Diagnostics;
using System.Text;

namespace PhantomManager;

internal static class CommandRunner
{
    public static async Task<CommandResult> RunAsync(
        string fileName,
        IReadOnlyList<string> args,
        string workingDirectory,
        Action<string>? log = null)
    {
        var result = await TryRunAsync(fileName, args, workingDirectory, log);
        if (result.ExitCode != 0)
        {
            throw new InvalidOperationException($"{fileName} {string.Join(" ", args)} failed: exit code {result.ExitCode}");
        }

        return result;
    }

    public static async Task<CommandResult> TryRunAsync(
        string fileName,
        IReadOnlyList<string> args,
        string workingDirectory,
        Action<string>? log = null)
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

        log?.Invoke($"> {fileName} {string.Join(" ", args)}");

        try
        {
            using var process = new Process { StartInfo = psi, EnableRaisingEvents = true };
            process.OutputDataReceived += (_, e) =>
            {
                if (e.Data is null)
                {
                    return;
                }

                output.AppendLine(e.Data);
                log?.Invoke(e.Data);
            };
            process.ErrorDataReceived += (_, e) =>
            {
                if (e.Data is null)
                {
                    return;
                }

                output.AppendLine(e.Data);
                log?.Invoke(e.Data);
            };

            process.Start();
            process.BeginOutputReadLine();
            process.BeginErrorReadLine();
            await process.WaitForExitAsync();
            return new CommandResult(process.ExitCode, output.ToString());
        }
        catch (Win32Exception ex)
        {
            log?.Invoke(ex.Message);
            return new CommandResult(9009, ex.Message);
        }
    }

    public static int RunQuiet(string fileName, IReadOnlyList<string> args, string workingDirectory)
    {
        try
        {
            using var process = StartQuiet(fileName, args, workingDirectory);
            if (process is null)
            {
                return -1;
            }

            if (!process.WaitForExit(10000))
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

    public static string? CaptureQuiet(string fileName, IReadOnlyList<string> args, string workingDirectory)
    {
        try
        {
            using var process = StartQuiet(fileName, args, workingDirectory);
            if (process is null)
            {
                return null;
            }

            var output = process.StandardOutput.ReadToEnd();
            if (!process.WaitForExit(10000))
            {
                process.Kill();
                return null;
            }
            return process.ExitCode == 0 ? output : null;
        }
        catch
        {
            return null;
        }
    }

    private static Process? StartQuiet(string fileName, IReadOnlyList<string> args, string workingDirectory)
    {
        var startInfo = new ProcessStartInfo(fileName)
        {
            WorkingDirectory = workingDirectory,
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

        return Process.Start(startInfo);
    }
}

internal sealed record CommandResult(int ExitCode, string Output);
