using System.Net;
using System.Net.NetworkInformation;
using System.Net.Sockets;

namespace PhantomManager;

internal static class NetworkAddressProvider
{
    public static string? GetPreferredLocalIPv4Address()
    {
        var interfaces = NetworkInterface.GetAllNetworkInterfaces()
            .Where(networkInterface =>
                networkInterface.OperationalStatus == OperationalStatus.Up
                && !networkInterface.Name.Contains("vEthernet", StringComparison.OrdinalIgnoreCase)
                && !networkInterface.Description.Contains("Hyper-V", StringComparison.OrdinalIgnoreCase)
                && (networkInterface.NetworkInterfaceType == NetworkInterfaceType.Ethernet
                    || networkInterface.NetworkInterfaceType == NetworkInterfaceType.Wireless80211))
            .OrderBy(networkInterface => networkInterface.NetworkInterfaceType == NetworkInterfaceType.Ethernet ? 0 : 1);

        foreach (var networkInterface in interfaces)
        {
            var address = networkInterface.GetIPProperties().UnicastAddresses
                .FirstOrDefault(unicast =>
                    unicast.Address.AddressFamily == AddressFamily.InterNetwork
                    && !IPAddress.IsLoopback(unicast.Address)
                    && !unicast.Address.ToString().StartsWith("169.254.", StringComparison.Ordinal));
            if (address is not null)
            {
                return address.Address.ToString();
            }
        }

        return null;
    }
}
