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
            "chcp 65001 >nul",
            $"set \"ORIG={sourceDir}\"",
            $"set \"DATA_DIR={destinationDir}\"",
            $"robocopy \"%ORIG%\" \"%DATA_DIR%\" /E /LOG:\"{mirrorLogPath}\"",
            "exit /b %ERRORLEVEL%",
            "",
        });

        File.WriteAllText(AppPaths.MirrorBatPath, batch, new UTF8Encoding(encoderShouldEmitUTF8Identifier: true));
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
