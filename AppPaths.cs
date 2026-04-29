namespace PhantomManager;

internal static class AppPaths
{
    public static string DefaultReleaseDir => "~/phantom/phantom-release";
    public static string LogDir => Path.Combine(AppContext.BaseDirectory, "log");
    public static string MirrorBatPath => Path.Combine(AppContext.BaseDirectory, "mirror.bat");
}
