using System.Text.RegularExpressions;

namespace PhantomManager;

internal sealed class ElasticsearchInitializer
{
    private const string ServiceName = "es";
    private const string EsUrl = "http://localhost:9200";
    private const string DefaultIndex = "patent-documents";
    private readonly string _releaseDir;

    public ElasticsearchInitializer(string releaseDir)
    {
        _releaseDir = releaseDir;
    }

    public async Task InitializeAsync(Action<string>? log)
    {
        var mappingPath = Path.Combine(_releaseDir, "es", "mapping.json");
        if (!File.Exists(mappingPath))
        {
            throw new FileNotFoundException("mapping.json が見つかりません。", mappingPath);
        }

        var indexName = ReadEnvValue("ES_INDEX") ?? DefaultIndex;
        log?.Invoke("Elasticsearch の起動を待機しています...");
        await WaitForElasticsearchAsync(log);

        log?.Invoke($"インデックスを削除しています: {indexName}");
        await CommandRunner.TryRunAsync(
            "docker",
            new[] { "compose", "exec", "-T", ServiceName, "curl", "-fsS", "-X", "DELETE", $"{EsUrl}/{indexName}" },
            _releaseDir,
            log);

        log?.Invoke("mapping.json をコンテナへコピーしています...");
        await CommandRunner.RunAsync(
            "docker",
            new[] { "compose", "cp", mappingPath, $"{ServiceName}:/tmp/mapping.json" },
            _releaseDir,
            log);

        log?.Invoke($"インデックスを作成しています: {indexName}");
        await CommandRunner.RunAsync(
            "docker",
            new[] { "compose", "exec", "-T", ServiceName, "curl", "-fsS", "-X", "PUT", $"{EsUrl}/{indexName}", "-H", "Content-Type: application/json", "-d", "@/tmp/mapping.json" },
            _releaseDir,
            log);

        log?.Invoke("データベース初期化が完了しました。");
    }

    private async Task WaitForElasticsearchAsync(Action<string>? log)
    {
        var startedAt = DateTimeOffset.UtcNow;
        while (DateTimeOffset.UtcNow - startedAt < TimeSpan.FromSeconds(60))
        {
            var result = await CommandRunner.TryRunAsync(
                "docker",
                new[] { "compose", "exec", "-T", ServiceName, "curl", "-fsS", $"{EsUrl}/_cluster/health?wait_for_status=yellow&timeout=1s" },
                _releaseDir);
            if (result.ExitCode == 0)
            {
                return;
            }

            await Task.Delay(TimeSpan.FromSeconds(2));
        }

        throw new TimeoutException("Elasticsearch が 60 秒以内に起動しませんでした。");
    }

    private string? ReadEnvValue(string name)
    {
        var envPath = Path.Combine(_releaseDir, ".env");
        if (!File.Exists(envPath))
        {
            return null;
        }

        var text = File.ReadAllText(envPath);
        var match = Regex.Match(text, $@"(?m)^{Regex.Escape(name)}=(.*)$");
        return match.Success ? match.Groups[1].Value.Trim().Trim('"') : null;
    }
}
