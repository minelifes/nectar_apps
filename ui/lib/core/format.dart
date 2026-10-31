import 'package:flutter/material.dart';

/// 1500 → "1.5 cores", 250 → "250m".
String formatCpu(int millicores) {
  if (millicores <= 0) return '0';
  if (millicores < 1000) return '${millicores}m';
  final cores = millicores / 1000;
  final s = cores == cores.roundToDouble()
      ? cores.toStringAsFixed(0)
      : cores.toStringAsFixed(1);
  return '$s ${cores == 1 ? 'core' : 'cores'}';
}

/// Bytes → "512 MB", "3.8 GB".
String formatBytes(int bytes) {
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  var v = bytes.toDouble();
  var i = 0;
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024;
    i++;
  }
  return '${v >= 10 || i == 0 ? v.toStringAsFixed(0) : v.toStringAsFixed(1)} ${units[i]}';
}

String formatMb(int mb) => formatBytes(mb * 1024 * 1024);

/// "just now", "5 min ago", "3 h ago", "2 d ago", then the date.
String timeAgo(DateTime? t, {DateTime? now}) {
  if (t == null) return '';
  final d = (now ?? DateTime.now()).difference(t);
  if (d.inSeconds < 45) return 'just now';
  if (d.inMinutes < 60) return '${d.inMinutes} min ago';
  if (d.inHours < 24) return '${d.inHours} h ago';
  if (d.inDays < 7) return '${d.inDays} d ago';
  return '${t.year}-${_two(t.month)}-${_two(t.day)}';
}

String _two(int n) => n.toString().padLeft(2, '0');

/// Display name and icon for a runtime id from the catalog.
({String label, IconData icon}) runtimeInfo(String id) => switch (id) {
  'java' => (label: 'Java', icon: Icons.coffee_rounded),
  'native' => (label: 'Native', icon: Icons.memory_rounded),
  'static' => (label: 'Static site', icon: Icons.web_rounded),
  'node' => (label: 'Node.js', icon: Icons.hub_rounded),
  'python' => (label: 'Python', icon: Icons.code_rounded),
  'php' => (label: 'PHP', icon: Icons.data_object_rounded),
  'image' => (label: 'Docker image', icon: Icons.inventory_2_rounded),
  _ => (label: id.isEmpty ? 'App' : id, icon: Icons.apps_rounded),
};

/// "app.deploy" → "deployed", for the activity feed.
String describeAction(String action) => switch (action) {
  'app.create' => 'created app',
  'app.deploy' => 'deployed',
  'app.rollback' => 'rolled back',
  'app.update' => 'changed settings of',
  'app.delete' => 'deleted app',
  'app.run' => 'ran task',
  'artifact.upload' => 'uploaded an artifact for',
  'env.update' => 'changed environment of',
  'env.reveal' => 'revealed secrets of',
  'domain.create' => 'added domain',
  'domain.update' => 'changed domain',
  'domain.delete' => 'removed domain',
  'domain.certificate' => 'requested a certificate for',
  'project.create' => 'created project',
  'project.update' => 'changed project',
  'project.delete' => 'deleted project',
  'project.adopt' => 'adopted namespace as',
  'user.create' => 'invited',
  'user.update' => 'updated user',
  'user.delete' => 'removed user',
  'user.bindings' => 'changed roles of',
  'role.create' => 'created role',
  'role.update' => 'changed role',
  'role.delete' => 'deleted role',
  'settings.update' => 'changed settings',
  'gateway.settings' => 'changed gateway settings',
  'gateway.unblock' => 'unblocked',
  'auth.login' => 'signed in',
  'setup.owner_created' => 'set up the panel',
  _ => action,
};
