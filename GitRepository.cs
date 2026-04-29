namespace PhantomManager;

internal sealed class GitRepository
{
    private readonly string _path;

    public GitRepository(string path)
    {
        _path = path;
    }

    public bool IsReady()
    {
        var path = WslCommand.PathArg(_path);
        return WslCommand.RunBashQuiet($"test -d {path}/.git && test -f {path}/docker-compose.yml") == 0;
    }

    public bool DirectoryExists()
    {
        return WslCommand.RunBashQuiet($"test -d {WslCommand.PathArg(_path)}") == 0;
    }

    public async Task CloneAsync(string repositoryUrl, Action<string>? log)
    {
        var parent = WslCommand.ParentPathArg(_path);
        var path = WslCommand.PathArg(_path);
        await WslCommand.RunBashAsync(
            $"mkdir -p {parent} && git clone {WslCommand.Quote(repositoryUrl)} {path}",
            log);
    }

    public async Task<string[]> GetTagsAsync(bool fetch, Action<string>? log)
    {
        if (!IsReady())
        {
            return Array.Empty<string>();
        }

        if (fetch)
        {
            await WslCommand.RunBashAsync($"cd {WslCommand.PathArg(_path)} && git fetch --tags --prune", log);
        }

        var result = await WslCommand.RunBashAsync($"cd {WslCommand.PathArg(_path)} && git tag --list --sort=-v:refname", log);
        return result.Output.Split(new[] { '\r', '\n' }, StringSplitOptions.RemoveEmptyEntries);
    }

    public async Task CheckoutTagAsync(string tag, Action<string>? log)
    {
        await WslCommand.RunBashAsync($"cd {WslCommand.PathArg(_path)} && git checkout {WslCommand.Quote(tag)}", log);
    }

    public string? GetCheckedOutTag()
    {
        if (!IsReady())
        {
            return null;
        }

        var isDetached = WslCommand.RunBashQuiet($"cd {WslCommand.PathArg(_path)} && git symbolic-ref -q HEAD") != 0;
        if (!isDetached)
        {
            return null;
        }

        var tag = WslCommand.CaptureBashQuiet($"cd {WslCommand.PathArg(_path)} && git describe --exact-match --tags HEAD");
        return string.IsNullOrWhiteSpace(tag) ? null : tag.Trim();
    }
}
