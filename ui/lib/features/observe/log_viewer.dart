import 'dart:async';
import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:url_launcher/url_launcher.dart';
import 'package:web_socket_channel/web_socket_channel.dart';

import '../../api/client.dart';
import '../../api/models.dart';
import '../../api/session.dart';
import '../../generated/l10n.dart';
import '../../theme/theme.dart';
import '../../theme/tokens.dart';
import '../../widgets/common.dart';

const _maxLines = 5000;

/// Live logs of all pods of an app (or one pod) over a WebSocket: follows new pods after deploys,
/// pause, search, wrap, copy and download.
class LogViewer extends ConsumerStatefulWidget {
  const LogViewer({super.key, required this.path, this.height = 520});

  /// API path of the workload: `/apps/5` or `/addons/3`.
  final String path;
  final double height;

  @override
  ConsumerState<LogViewer> createState() => _LogViewerState();
}

class _LogViewerState extends ConsumerState<LogViewer> {
  final List<LogLine> _lines = [];
  final List<LogLine> _held = []; // received while paused
  List<PodBrief> _pods = const [];
  String? _pod;
  String _filter = '';
  bool _paused = false;
  bool _wrap = true;
  bool _connected = false;
  String? _error;
  final _scroll = ScrollController();
  bool _stickToBottom = true;

  WebSocketChannel? _ch;
  StreamSubscription<dynamic>? _sub;
  Timer? _retry;
  bool _disposed = false;

  @override
  void initState() {
    super.initState();
    _scroll.addListener(() {
      if (!_scroll.hasClients) return;
      _stickToBottom = _scroll.position.pixels >= _scroll.position.maxScrollExtent - 40;
    });
    _connect();
  }

  @override
  void dispose() {
    _disposed = true;
    _retry?.cancel();
    _sub?.cancel();
    _ch?.sink.close();
    _scroll.dispose();
    super.dispose();
  }

  Future<void> _connect() async {
    _retry?.cancel();
    await _sub?.cancel();
    await _ch?.sink.close();
    final api = ref.read(apiProvider);
    try {
      // also renews an expired access token before it goes into the WebSocket URL
      final pods = listOf(await api.get('${widget.path}/pods'), PodBrief.fromJson);
      if (!mounted) return;
      setState(() {
        _pods = pods;
        _error = null;
      });
      final ch = WebSocketChannel.connect(api.wsUri('${widget.path}/logs/ws', {'tail': '200', 'pod': ?_pod}));
      _ch = ch;
      await ch.ready;
      if (!mounted) {
        await ch.sink.close();
        return;
      }
      setState(() => _connected = true);
      _sub = ch.stream.listen(_onMessage, onError: (Object e) => _lost(describeError(e)), onDone: () => _lost(null));
    } on Object catch (e) {
      _lost(describeError(e));
    }
  }

  void _lost(String? error) {
    if (_disposed) return;
    setState(() {
      _connected = false;
      _error = error;
    });
    _retry = Timer(const Duration(seconds: 3), _connect);
  }

  void _onMessage(dynamic data) {
    if (data is! String) return;
    final Object? decoded;
    try {
      decoded = jsonDecode(data);
    } on FormatException {
      return;
    }
    if (decoded is! Map) return;
    final msg = decoded; // promoted, also inside the closures below
    switch (msg['type']) {
      case 'line':
        final line = LogLine.fromJson(msg);
        if (_paused) {
          _held.add(line);
          if (_held.length > _maxLines) _held.removeRange(0, _held.length - _maxLines);
          setState(() {});
        } else {
          _add([line]);
        }
      case 'pods':
        setState(() => _pods = listOf(msg['pods'], PodBrief.fromJson));
      case 'info':
        setState(() => _error = '${msg['message']}');
    }
  }

  void _add(List<LogLine> lines) {
    setState(() {
      _lines.addAll(lines);
      if (_lines.length > _maxLines) _lines.removeRange(0, _lines.length - _maxLines);
    });
    if (_stickToBottom) {
      WidgetsBinding.instance.addPostFrameCallback((_) {
        if (_scroll.hasClients) _scroll.jumpTo(_scroll.position.maxScrollExtent);
      });
    }
  }

  void _togglePause() {
    if (_paused) {
      final held = List<LogLine>.of(_held);
      _held.clear();
      setState(() => _paused = false);
      _add(held);
    } else {
      setState(() => _paused = true);
    }
  }

  void _selectPod(String? pod) {
    setState(() {
      _pod = pod;
      _lines.clear();
      _held.clear();
    });
    _connect();
  }

  List<LogLine> get _visible {
    final f = _filter.toLowerCase();
    return f.isEmpty ? _lines : _lines.where((l) => l.line.toLowerCase().contains(f)).toList();
  }

  String _asText(List<LogLine> lines) => lines.map((l) => '${l.t?.toIso8601String() ?? ''} ${l.pod} ${l.line}').join('\n');

  @override
  Widget build(BuildContext context) {
    final s = S.of(context);
    final c = NectarColors.of(context);
    final visible = _visible;
    final api = ref.watch(apiProvider);
    final podColors = <String, Color>{};
    final palette = [NectarTokens.amber, c.deploying, c.running, NectarTokens.pollen, c.failed, c.muted];
    for (var i = 0; i < _pods.length; i++) {
      podColors[_pods[i].name] = palette[i % palette.length];
    }
    final multiPod = _pod == null && _pods.length > 1;

    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        Wrap(
          spacing: 8,
          runSpacing: 8,
          crossAxisAlignment: WrapCrossAlignment.center,
          children: [
            DropdownButton<String?>(
              value: _pods.any((p) => p.name == _pod) ? _pod : null,
              underline: const SizedBox.shrink(),
              items: [
                DropdownMenuItem<String?>(value: null, child: Text(s.observeAllPods)),
                for (final p in _pods)
                  DropdownMenuItem<String?>(
                    value: p.name,
                    child: Text('${p.name}${p.restarts > 0 ? '  (${p.restarts} restarts)' : ''}', style: monoStyle(context, size: 12.5)),
                  ),
              ],
              onChanged: _selectPod,
            ),
            SizedBox(
              width: 220,
              child: TextField(
                decoration: InputDecoration(isDense: true, prefixIcon: Icon(Icons.search_rounded, size: 18), hintText: s.observeFilter),
                onChanged: (v) => setState(() => _filter = v.trim()),
              ),
            ),
            OutlinedButton.icon(
              onPressed: _togglePause,
              icon: Icon(_paused ? Icons.play_arrow_rounded : Icons.pause_rounded, size: 18),
              label: Text(_paused ? 'Resume${_held.isEmpty ? '' : ' (${_held.length} new)'}' : 'Pause'),
            ),
            IconButton(
              tooltip: _wrap ? 'No wrapping' : 'Wrap lines',
              icon: Icon(_wrap ? Icons.wrap_text_rounded : Icons.notes_rounded, size: 20),
              onPressed: () => setState(() => _wrap = !_wrap),
            ),
            IconButton(
              tooltip: s.observeCopyShownLines,
              icon: const Icon(Icons.copy_rounded, size: 18),
              onPressed: visible.isEmpty
                  ? null
                  : () {
                      Clipboard.setData(ClipboardData(text: _asText(visible)));
                      showSnack(context, s.observeCopiedLines2(visible.length));
                    },
            ),
            if (!api.useTokens) // the web build served by the panel: a plain download with the session cookie
              IconButton(
                tooltip: s.observeDownloadLast5000Lines,
                icon: const Icon(Icons.download_rounded, size: 20),
                onPressed: () => launchUrl(Uri.parse(
                  '${api.baseUrl}/api${widget.path}/logs?format=text&tail=5000${_pod == null ? '' : '&pod=$_pod'}',
                )),
              ),
            IconButton(
              tooltip: s.observeClear,
              icon: const Icon(Icons.clear_all_rounded, size: 20),
              onPressed: () => setState(() {
                _lines.clear();
                _held.clear();
              }),
            ),
            Row(
              mainAxisSize: MainAxisSize.min,
              children: [
                Container(
                  width: 8,
                  height: 8,
                  decoration: BoxDecoration(shape: BoxShape.circle, color: _connected ? c.running : c.stopped),
                ),
                const SizedBox(width: 6),
                Text(_connected ? (_paused ? 'paused' : 'live') : 'reconnecting…', style: TextStyle(color: c.muted, fontSize: 12)),
              ],
            ),
          ],
        ),
        if (_error != null)
          Padding(padding: const EdgeInsets.only(top: 6), child: Text(_error!, style: TextStyle(color: c.warning, fontSize: 12.5))),
        const SizedBox(height: 10),
        Container(
          height: widget.height,
          decoration: BoxDecoration(
            color: const Color(0xFF0A0B0D),
            borderRadius: BorderRadius.circular(NectarTokens.radius),
            border: Border.all(color: c.border),
          ),
          child: visible.isEmpty
              ? Center(
                  child: Text(
                    _lines.isEmpty ? (_pods.isEmpty ? 'No pods are running.' : 'Waiting for output…') : 'No line matches the filter.',
                    style: const TextStyle(color: Color(0xFF8A8F98), fontSize: 13),
                  ),
                )
              : Scrollbar(
                  controller: _scroll,
                  child: ListView.builder(
                    controller: _scroll,
                    padding: const EdgeInsets.all(10),
                    itemCount: visible.length,
                    itemBuilder: (context, i) {
                      final l = visible[i];
                      final text = Text.rich(
                        TextSpan(
                          children: [
                            if (l.t != null)
                              TextSpan(text: '${_hms(l.t!)} ', style: const TextStyle(color: Color(0xFF5C616B))),
                            if (multiPod)
                              TextSpan(text: '${_short(l.pod)} ', style: TextStyle(color: podColors[l.pod] ?? NectarTokens.amber)),
                            TextSpan(text: l.line),
                          ],
                        ),
                        softWrap: _wrap,
                        overflow: _wrap ? TextOverflow.visible : TextOverflow.clip,
                        style: monoStyle(context, size: 12.5, color: const Color(0xFFD9DBE0)),
                      );
                      return _wrap ? text : SingleChildScrollView(scrollDirection: Axis.horizontal, child: text);
                    },
                  ),
                ),
        ),
      ],
    );
  }

  static String _hms(DateTime t) {
    final l = t.toLocal();
    String two(int n) => n.toString().padLeft(2, '0');
    return '${two(l.hour)}:${two(l.minute)}:${two(l.second)}';
  }

  /// "api-7c9d8f6b5-x2k4p" → "x2k4p"
  static String _short(String pod) {
    final i = pod.lastIndexOf('-');
    return i < 0 ? pod : pod.substring(i + 1);
  }
}

/// The output of the previous (crashed) container of a pod.
Future<void> showCrashLog(BuildContext context, WidgetRef ref, String path, String pod) => showDialog<void>(
      context: context,
      builder: (ctx) => AlertDialog(
        title: Text(S.of(context).observeBeforeTheLastRestart(pod)),
        content: SizedBox(
          width: 760,
          height: 440,
          child: FutureBuilder<List<LogLine>>(
            future: ref
                .read(apiProvider)
                .get('$path/logs', query: {'pod': pod, 'previous': '1', 'tail': 300}).then((v) => listOf(v, LogLine.fromJson)),
            builder: (ctx, snap) {
              if (snap.hasError) return ErrorBlock(error: snap.error!);
              if (!snap.hasData) return const LoadingBlock();
              final lines = snap.data!;
              if (lines.isEmpty) return Center(child: Text(S.of(context).observeThePreviousContainerWroteNothing));
              return Container(
                padding: const EdgeInsets.all(10),
                decoration: BoxDecoration(color: const Color(0xFF0A0B0D), borderRadius: BorderRadius.circular(NectarTokens.radius)),
                child: SingleChildScrollView(
                  reverse: true,
                  child: SelectableText(
                    lines.map((l) => l.line).join('\n'),
                    style: monoStyle(ctx, size: 12.5, color: const Color(0xFFD9DBE0)),
                  ),
                ),
              );
            },
          ),
        ),
        actions: [TextButton(onPressed: () => Navigator.pop(ctx), child: Text(S.of(context).actionClose))],
      ),
    );
