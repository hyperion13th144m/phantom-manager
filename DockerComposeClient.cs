using System.Text.Json;

namespace PhantomManager;

internal sealed class DockerComposeClient
{
    private readonly string _workingDirectory;

    public DockerComposeClient(string workingDirectory)
    {
        _workingDirectory = workingDirectory;
    }

    public async Task UpAsync(Action<string>? log, bool useSsl = false)
    {
        var files = useSsl ? "-f docker-compose.yml -f docker-compose.secure.yml " : "";
        await WslCommand.RunBashAsync($"cd {WslCommand.PathArg(_workingDirectory)} && {DockerCli.WslDockerArg} compose {files}up -d", log);
    }

    public async Task DownAsync(Action<string>? log)
    {
        await WslCommand.RunBashAsync($"cd {WslCommand.PathArg(_workingDirectory)} && {DockerCli.WslDockerArg} compose down", log);
    }

    public async Task<IReadOnlyList<ComposeService>> GetServicesAsync(Action<string>? log)
    {
        var result = await WslCommand.TryBashAsync(
            $"cd {WslCommand.PathArg(_workingDirectory)} && {DockerCli.WslDockerArg} compose ps --all --format json",
            log);
        if (result.ExitCode != 0)
        {
            return Array.Empty<ComposeService>();
        }

        var services = new List<ComposeService>();
        foreach (var line in result.Output.Split(new[] { '\r', '\n' }, StringSplitOptions.RemoveEmptyEntries))
        {
            using var document = JsonDocument.Parse(line);
            var service = document.RootElement;
            services.Add(new ComposeService(
                GetJsonString(service, "Service"),
                GetJsonString(service, "State"),
                GetJsonString(service, "Status"),
                FormatPorts(service)));
        }

        return services;
    }

    private static string GetJsonString(JsonElement element, string name)
    {
        return element.TryGetProperty(name, out var value) ? value.ToString() : "";
    }

    private static string FormatPorts(JsonElement service)
    {
        var ports = GetJsonString(service, "Ports");
        return string.IsNullOrWhiteSpace(ports) ? FormatPublishers(service) : ports;
    }

    private static string FormatPublishers(JsonElement service)
    {
        if (!service.TryGetProperty("Publishers", out var publishers) || publishers.ValueKind != JsonValueKind.Array)
        {
            return "";
        }

        var parts = new List<string>();
        foreach (var publisher in publishers.EnumerateArray())
        {
            var target = GetJsonString(publisher, "TargetPort");
            var published = GetJsonString(publisher, "PublishedPort");
            var protocol = GetJsonString(publisher, "Protocol");
            if (!string.IsNullOrWhiteSpace(published))
            {
                parts.Add($"{published}->{target}/{protocol}");
            }
            else if (!string.IsNullOrWhiteSpace(target))
            {
                parts.Add($"{target}/{protocol}");
            }
        }

        return string.Join(", ", parts);
    }
}

internal sealed record ComposeService(string Name, string State, string Status, string Ports)
{
    public bool IsRunning => string.Equals(State, "running", StringComparison.OrdinalIgnoreCase);
}
