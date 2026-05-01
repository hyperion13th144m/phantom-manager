using System.Text;

namespace PhantomManager;

internal static class MirrorBatchWriter
{
    public static void Create(string origDir, string dataDir)
    {
        Directory.CreateDirectory(AppPaths.LogDir);

        var mirrorLogPath = Path.Combine(AppPaths.LogDir, "mirror.log");
        var sourceDir = NormalizeRobocopyPath(origDir);
        var destinationDir = NormalizeRobocopyPath(dataDir);
        var batch = string.Join(Environment.NewLine, new[]
        {
            "@echo off",
            $"set \"ORIG={sourceDir}\"",
            $"set \"DATA_DIR={destinationDir}\"",
            $"robocopy \"%ORIG%\" \"%DATA_DIR%\" \"*AAA.JWX\" \"*AAA.JPC\" \"*NNF.JWX\" \"*NNF.JPC\" /E /LOG:\"{mirrorLogPath}\" /TEE",
            "exit /b %ERRORLEVEL%",
            "",
        });

        Encoding.RegisterProvider(CodePagesEncodingProvider.Instance);
        Encoding sjis = Encoding.GetEncoding("Shift_JIS");
        File.WriteAllText(AppPaths.MirrorBatPath, batch, sjis);
    }

    private static string NormalizeRobocopyPath(string path)
    {
        var trimmed = path.Trim();
        var root = Path.GetPathRoot(trimmed);
        if (!string.IsNullOrWhiteSpace(root)
            && string.Equals(trimmed.TrimEnd(Path.DirectorySeparatorChar, Path.AltDirectorySeparatorChar), root.TrimEnd(Path.DirectorySeparatorChar, Path.AltDirectorySeparatorChar), StringComparison.OrdinalIgnoreCase))
        {
            return root.TrimEnd(Path.DirectorySeparatorChar, Path.AltDirectorySeparatorChar) + Path.DirectorySeparatorChar + ".";
        }

        return trimmed.TrimEnd(Path.DirectorySeparatorChar, Path.AltDirectorySeparatorChar);
    }
}
