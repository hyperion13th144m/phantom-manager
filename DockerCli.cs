namespace PhantomManager;

internal static class DockerCli
{
    private const string WslDockerDesktopPath = "/mnt/c/Program Files/Docker/Docker/resources/bin/docker.exe";

    public static string WslDockerArg => WslCommand.Quote(WslDockerDesktopPath);

    public static string WindowsDockerPath
    {
        get
        {
            var dockerDesktopCli = Path.Combine(
                Environment.GetFolderPath(Environment.SpecialFolder.ProgramFiles),
                "Docker",
                "Docker",
                "resources",
                "bin",
                "docker.exe");
            return File.Exists(dockerDesktopCli) ? dockerDesktopCli : "docker";
        }
    }
}
