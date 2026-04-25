using System.Text;

namespace PhantomManager;

internal static class MirrorBatchWriter
{
    public static void Create(string origDir, string dataDir)
    {
        Directory.CreateDirectory(AppPaths.LogDir);
        Directory.CreateDirectory(AppPaths.BatDir);

        var mirrorLogPath = Path.Combine(AppPaths.LogDir, "mirror.log");
        var batch = string.Join(Environment.NewLine, new[]
        {
            "@echo off",
            "chcp 65001 >nul",
            $"set \"ORIG={origDir}\"",
            $"set \"DATA_DIR={dataDir}\"",
            $"robocopy \"%ORIG%\" \"%DATA_DIR%\" /E /LOG:\"{mirrorLogPath}\"",
            "exit /b %ERRORLEVEL%",
            "",
        });

        File.WriteAllText(AppPaths.MirrorBatPath, batch, new UTF8Encoding(encoderShouldEmitUTF8Identifier: true));
    }
}
