import 'dart:async';
import 'dart:typed_data';

import 'package:dio/dio.dart';
import 'package:file_picker/file_picker.dart';
import 'package:flutter/foundation.dart';

import '../../api/client.dart';
import '../../api/models.dart';

/// A file chosen for upload. Native platforms stream it from disk; the web reads it into memory.
class PickedArtifact {
  PickedArtifact({required this.name, required this.size, Uint8List? bytes, Stream<List<int>>? stream})
      : _bytes = bytes,
        _stream = stream;

  final String name;
  final int size;
  final Uint8List? _bytes;
  Stream<List<int>>? _stream;

  /// The file content. A disk stream can be read once; bytes can be re-sent (retry).
  Stream<List<int>> open() {
    final bytes = _bytes;
    if (bytes != null) return _chunks(bytes);
    final s = _stream;
    if (s == null) throw StateError('the file was already sent — choose it again');
    _stream = null;
    return s;
  }

  bool get canRetry => _bytes != null;

  static Stream<List<int>> _chunks(Uint8List bytes) async* {
    const chunk = 256 * 1024;
    for (var i = 0; i < bytes.length; i += chunk) {
      yield bytes.sublist(i, i + chunk > bytes.length ? bytes.length : i + chunk);
    }
  }
}

/// File extensions the picker should offer for a runtime (".tar.gz" → "gz"; "*" → any file).
List<String>? pickerExtensions(Runtime rt) {
  if (rt.accept.isEmpty || rt.accept.contains('*')) return null;
  return {for (final a in rt.accept) a.split('.').last.toLowerCase()}.toList();
}

/// Opens the system file picker. Returns null when the user cancels.
Future<PickedArtifact?> pickArtifact(Runtime rt) async {
  final exts = pickerExtensions(rt);
  final res = await FilePicker.platform.pickFiles(
    type: exts == null ? FileType.any : FileType.custom,
    allowedExtensions: exts,
    withData: kIsWeb,
    withReadStream: !kIsWeb,
  );
  final f = res?.files.single;
  if (f == null) return null;
  return PickedArtifact(name: f.name, size: f.size, bytes: f.bytes, stream: f.readStream);
}

/// Checks a file name against the runtime's accepted types before uploading.
String? artifactProblem(Runtime rt, String name) {
  final lower = name.toLowerCase();
  if (rt.accept.isEmpty || rt.accept.contains('*')) return null;
  if (rt.accept.any((a) => lower.endsWith(a.toLowerCase()))) return null;
  return '${rt.name} apps take ${rt.accept.join(', ')} files.';
}

/// Uploads [file] as an artifact of app [appId] with progress (0–1).
Future<Artifact> uploadArtifact(
  Api api,
  int appId,
  PickedArtifact file, {
  required void Function(double progress) onProgress,
  CancelToken? cancel,
}) async {
  final res = await api.upload(
    '/apps/$appId/artifacts',
    data: file.open(),
    length: file.size,
    query: {'filename': file.name},
    cancel: cancel,
    onProgress: (sent, total) => onProgress(total <= 0 ? 0 : sent / total),
  );
  return Artifact.fromJson(res);
}
