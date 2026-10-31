/// Resource presets of the New App wizard (PLAN.md §9).
class ResourcePreset {
  const ResourcePreset(this.id, this.label, this.cpuMillicores, this.memoryMb);

  final String id;
  final String label;
  final int cpuMillicores;
  final int memoryMb;
}

const resourcePresets = [
  ResourcePreset('nano', 'Nano', 250, 256),
  ResourcePreset('small', 'Small', 500, 512),
  ResourcePreset('medium', 'Medium', 1000, 1024),
  ResourcePreset('large', 'Large', 2000, 2048),
];

/// The preset matching cpu/memory, or null (custom).
ResourcePreset? presetFor(int cpu, int mem) {
  for (final p in resourcePresets) {
    if (p.cpuMillicores == cpu && p.memoryMb == mem) return p;
  }
  return null;
}

final envKeyRe = RegExp(r'^[A-Za-z_][A-Za-z0-9_]*$');

/// Parses a .env file: KEY=value lines, `export KEY=…`, quotes, comments. Returns (key, value) pairs in order;
/// later duplicates win.
List<(String, String)> parseDotEnv(String text) {
  final out = <String, String>{};
  for (var line in text.split(RegExp(r'\r?\n'))) {
    line = line.trim();
    if (line.isEmpty || line.startsWith('#')) continue;
    if (line.startsWith('export ')) line = line.substring(7).trimLeft();
    final eq = line.indexOf('=');
    if (eq <= 0) continue;
    final key = line.substring(0, eq).trim();
    if (!envKeyRe.hasMatch(key)) continue;
    var value = line.substring(eq + 1).trim();
    if (value.length >= 2 &&
        ((value.startsWith('"') && value.endsWith('"')) ||
            (value.startsWith("'") && value.endsWith("'")))) {
      final quote = value[0];
      value = value.substring(1, value.length - 1);
      if (quote == '"') {
        value = value.replaceAll(r'\n', '\n').replaceAll(r'\"', '"');
      }
    } else {
      final hash = value.indexOf(' #');
      if (hash >= 0) value = value.substring(0, hash).trimRight();
    }
    out.remove(key);
    out[key] = value;
  }
  return [for (final e in out.entries) (e.key, e.value)];
}

/// Heuristic: values of keys like these are stored as secrets by default.
bool looksSecret(String key) => RegExp(
  r'(PASS|SECRET|TOKEN|KEY|PRIVATE|CREDENTIAL|DSN|DATABASE_URL)',
  caseSensitive: false,
).hasMatch(key);
