namespace PhantomManager;

internal static class AppPaths
{
    public static string DefaultReleaseDir => Path.Combine(AppContext.BaseDirectory, "phantom-release");
    public static string DefaultSrcDir => Path.Combine(AppContext.BaseDirectory, "インターネット出願ソフトのデータ");
    public static string LogDir => Path.Combine(AppContext.BaseDirectory, "log");
    public static string BatDir => Path.Combine(AppContext.BaseDirectory, "bat");
    public static string MirrorBatPath => Path.Combine(BatDir, "mirror.bat");
}
