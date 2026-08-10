namespace PhantomManager;

internal sealed class ElasticsearchInitializer
{
    private const string ServiceName = "es";
    private const string EsUrl = "http://localhost:9200";
    private readonly string _releaseDir;

    public ElasticsearchInitializer(string releaseDir)
    {
        _releaseDir = releaseDir;
    }

    public async Task InitializeAsync(Action<string>? log)
    {
        log?.Invoke("Elasticsearch の起動を待機しています...");
        await WaitForElasticsearchAsync(log);

        log?.Invoke("Elasticsearch 初期化スクリプトを実行しています...");
        await WslCommand.RunBashAsync(
            $"cd {WslCommand.PathArg(_releaseDir)} && {DockerCli.WslDockerArg} compose exec {ServiceName} /init.sh -f",
            log);

        log?.Invoke("データベース初期化が完了しました。");
    }

    private async Task WaitForElasticsearchAsync(Action<string>? log)
    {
        var startedAt = DateTimeOffset.UtcNow;
        while (DateTimeOffset.UtcNow - startedAt < TimeSpan.FromSeconds(60))
        {
            var result = await WslCommand.TryBashAsync(
                $"cd {WslCommand.PathArg(_releaseDir)} && {DockerCli.WslDockerArg} compose exec -T {ServiceName} curl -fsS {WslCommand.Quote($"{EsUrl}/_cluster/health?wait_for_status=yellow&timeout=1s")}");
            if (result.ExitCode == 0)
            {
                return;
            }

            await Task.Delay(TimeSpan.FromSeconds(2));
        }

        throw new TimeoutException("Elasticsearch が 60 秒以内に起動しませんでした。");
    }
}
