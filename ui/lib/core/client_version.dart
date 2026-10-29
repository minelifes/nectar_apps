/// Version of this app build (keep in sync with `version:` in pubspec.yaml; CI checks it).
const clientVersion = '0.1.0';

/// Compares two semantic versions ("1.2.3", "v1.2.3-beta.1"); unparseable versions compare equal.
int compareVersions(String a, String b) {
  List<int>? parse(String v) {
    final core = v.trim().replaceFirst(RegExp(r'^v'), '').split(RegExp(r'[-+]')).first.split('.');
    if (core.length != 3) return null;
    final nums = core.map(int.tryParse).toList();
    return nums.contains(null) ? null : nums.cast<int>();
  }

  final pa = parse(a), pb = parse(b);
  if (pa == null || pb == null) return 0;
  for (var i = 0; i < 3; i++) {
    if (pa[i] != pb[i]) return pa[i] < pb[i] ? -1 : 1;
  }
  final preA = a.contains('-'), preB = b.contains('-');
  if (preA != preB) return preA ? -1 : 1;
  return 0;
}
