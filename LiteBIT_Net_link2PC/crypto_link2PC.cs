using System;
using System.Collections.Generic;
using System.Text;
using System.Security.Cryptography;

public static class SrunMachineCrypto
{
    private const string Alphabet = "LVoJPiCN2R8G90yg+hmFHuacZ1OWMnrsSTXkYpUq/3dlbfKwv6xztjI7DeBE45QA";
    private const string Standard = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/";

    private static uint[] Words(byte[] data, bool length)
    {
        uint[] words = new uint[(data.Length + 3) / 4 + (length ? 1 : 0)];
        for (int i = 0; i < data.Length; i++) words[i / 4] |= (uint)data[i] << ((i % 4) * 8);
        if (length) words[words.Length - 1] = (uint)data.Length;
        return words;
    }

    public static byte[] XEncode(string text, string token)
    {
        if (text.Length == 0) return new byte[0];
        uint[] v = Words(Encoding.UTF8.GetBytes(text), true);
        uint[] k = Words(Encoding.UTF8.GetBytes(token), false);
        if (k.Length < 4) Array.Resize(ref k, 4);
        int n = v.Length - 1;
        uint z = v[n], total = 0;
        int rounds = 6 + 52 / (n + 1);
        unchecked
        {
            while (rounds-- > 0)
            {
                total += 0x9E3779B9;
                uint e = (total >> 2) & 3;
                for (int p = 0; p <= n; p++)
                {
                    uint y = v[p == n ? 0 : p + 1];
                    uint m = (z >> 5) ^ (y << 2);
                    m += ((y >> 3) ^ (z << 4)) ^ (total ^ y);
                    m += k[(p & 3) ^ (int)e] ^ z;
                    v[p] += m;
                    z = v[p];
                }
            }
        }
        byte[] result = new byte[v.Length * 4];
        for (int i = 0; i < result.Length; i++) result[i] = (byte)(v[i / 4] >> ((i % 4) * 8));
        return result;
    }

    public static string CustomBase64(byte[] data)
    {
        char[] chars = Convert.ToBase64String(data).ToCharArray();
        for (int i = 0; i < chars.Length; i++)
        {
            int index = Standard.IndexOf(chars[i]);
            if (index >= 0) chars[i] = Alphabet[index];
        }
        return new string(chars);
    }

    private static string Quote(string text)
    {
        StringBuilder output = new StringBuilder("\"");
        foreach (char c in text)
        {
            if (c == '"' || c == '\\') output.Append('\\').Append(c);
            else if (c < 32) output.Append("\\u").Append(((int)c).ToString("x4"));
            else output.Append(c);
        }
        return output.Append('"').ToString();
    }

    private static string Hex(byte[] bytes)
    {
        return BitConverter.ToString(bytes).Replace("-", "").ToLowerInvariant();
    }

    public static Dictionary<string, string> LoginParameters(
        string username, string password, string ip, string token, string acid)
    {
        string json = "{\"username\":" + Quote(username) + ",\"password\":" + Quote(password)
            + ",\"ip\":" + Quote(ip) + ",\"acid\":" + Quote(acid) + ",\"enc_ver\":\"srun_bx1\"}";
        string info = "{SRBX1}" + CustomBase64(XEncode(json, token));
        string md5;
        using (HMACMD5 hash = new HMACMD5(Encoding.UTF8.GetBytes(token)))
            md5 = Hex(hash.ComputeHash(Encoding.UTF8.GetBytes(password)));
        string sumText = token + username + token + md5 + token + acid + token + ip
            + token + "200" + token + "1" + token + info;
        string checksum;
        using (SHA1 hash = SHA1.Create()) checksum = Hex(hash.ComputeHash(Encoding.UTF8.GetBytes(sumText)));
        return new Dictionary<string, string> {
            {"action", "login"}, {"username", username}, {"password", "{MD5}" + md5},
            {"ac_id", acid}, {"ip", ip}, {"chksum", checksum}, {"info", info},
            {"n", "200"}, {"type", "1"}, {"os", "Windows 10"}, {"name", "Windows"},
            {"double_stack", "0"}, {"ignore", "2"}
        };
    }
}
