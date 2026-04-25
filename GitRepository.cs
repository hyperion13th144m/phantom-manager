using System.Text;

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
        return Directory.Exists(_path)
            && Directory.Exists(Path.Combine(_path, ".git"))
            && File.Exists(Path.Combine(_path, "docker-compose.yml"));
    }

    public async Task<string[]> GetTagsAsync(bool fetch, Action<string>? log)
    {
        if (!IsReady())
        {
            return Array.Empty<string>();
        }

        if (fetch)
        {
            await CommandRunner.RunAsync("git", new[] { "fetch", "--tags", "--prune" }, _path, log);
        }

        var result = await CommandRunner.RunAsync("git", new[] { "tag", "--list", "--sort=-v:refname" }, _path, log);
        return result.Output.Split(new[] { '\r', '\n' }, StringSplitOptions.RemoveEmptyEntries);
    }

    public async Task CheckoutTagAsync(string tag, Action<string>? log)
    {
        await CommandRunner.RunAsync("git", new[] { "checkout", tag }, _path, log);
    }

    public string? GetCheckedOutTag()
    {
        if (!IsReady())
        {
            return null;
        }

        var isDetached = CommandRunner.RunQuiet("git", new[] { "symbolic-ref", "-q", "HEAD" }, _path) != 0;
        if (!isDetached)
        {
            return null;
        }

        var tag = CommandRunner.CaptureQuiet("git", new[] { "describe", "--exact-match", "--tags", "HEAD" }, _path);
        return string.IsNullOrWhiteSpace(tag) ? null : tag.Trim();
    }
}
