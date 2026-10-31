import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../api/client.dart';
import '../../api/models.dart';
import '../../api/session.dart';
import '../../generated/l10n.dart';
import '../../theme/theme.dart';
import '../../theme/tokens.dart';
import '../../widgets/common.dart';
import 'log_viewer.dart';

/// The Logs tab: the live stream, or the stored history with search.
class LogsPanel extends StatefulWidget {
  const LogsPanel({super.key, required this.app});

  final App app;

  @override
  State<LogsPanel> createState() => _LogsPanelState();
}

class _LogsPanelState extends State<LogsPanel> {
  bool _history = false;

  @override
  Widget build(BuildContext context) {
    final l = S.of(context);
    return Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Align(
            alignment: Alignment.centerLeft,
            child: SegmentedButton<bool>(
              segments: [
                ButtonSegment(value: false, label: Text(l.observeLive), icon: Icon(Icons.podcasts_rounded, size: 16)),
                ButtonSegment(value: true, label: Text(l.observeHistory), icon: Icon(Icons.history_rounded, size: 16)),
              ],
              selected: {_history},
              showSelectedIcon: false,
              onSelectionChanged: (s) => setState(() => _history = s.first),
            ),
          ),
          const SizedBox(height: 10),
          if (_history) LogHistoryView(app: widget.app) else LogViewer(key: ValueKey('logs-${widget.app.id}'), path: '/apps/${widget.app.id}'),
        ],
      );
  }
}

class _HLine {
  const _HLine(this.pod, this.time, this.line);

  final String pod;
  final DateTime time;
  final String line;
}

/// Searches the logs stored on the panel (every pod the app had, across restarts and deploys).
class LogHistoryView extends ConsumerStatefulWidget {
  const LogHistoryView({super.key, required this.app});

  final App app;

  @override
  ConsumerState<LogHistoryView> createState() => _LogHistoryViewState();
}

class _LogHistoryViewState extends ConsumerState<LogHistoryView> {
  final _query = TextEditingController();
  bool _regex = false;
  String _range = '24h';
  final _lines = <_HLine>[];
  bool _busy = false;
  bool _more = false;
  String? _next;
  String? _error;
  bool _searched = false;

  static const _ranges = {'1h': Duration(hours: 1), '6h': Duration(hours: 6), '24h': Duration(hours: 24), '7d': Duration(days: 7), 'all': Duration.zero};

  @override
  void initState() {
    super.initState();
    _search();
  }

  @override
  void dispose() {
    _query.dispose();
    super.dispose();
  }

  Future<void> _search({bool more = false}) async {
    setState(() {
      _busy = true;
      _error = null;
      if (!more) {
        _lines.clear();
        _next = null;
      }
    });
    final span = _ranges[_range]!;
    try {
      final res = await ref.read(apiProvider).get('/apps/${widget.app.id}/logs/history', query: {
        if (_query.text.trim().isNotEmpty) 'q': _query.text.trim(),
        if (_regex) 'regex': '1',
        if (span > Duration.zero) 'from': DateTime.now().toUtc().subtract(span).toIso8601String(),
        if (more && _next != null) 'to': _next,
        'limit': 500,
      });
      final m = res as Map;
      if (!mounted) return;
      setState(() {
        for (final l in (m['lines'] as List? ?? const [])) {
          final x = l as Map;
          _lines.add(_HLine('${x['pod']}', DateTime.tryParse('${x['t']}')?.toLocal() ?? DateTime.now(), '${x['line']}'));
        }
        _more = m['more'] == true;
        _next = m['next'] as String?;
        _searched = true;
      });
    } on Object catch (e) {
      if (mounted) setState(() => _error = describeError(e));
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  String _stamp(DateTime t) =>
      '${t.month.toString().padLeft(2, '0')}-${t.day.toString().padLeft(2, '0')} ${t.hour.toString().padLeft(2, '0')}:${t.minute.toString().padLeft(2, '0')}:${t.second.toString().padLeft(2, '0')}';

  @override
  Widget build(BuildContext context) {
        final s = S.of(context);
    final c = NectarColors.of(context);
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        Wrap(
          spacing: 10,
          runSpacing: 8,
          crossAxisAlignment: WrapCrossAlignment.center,
          children: [
            SizedBox(
              width: 320,
              child: TextField(
                controller: _query,
                style: monoStyle(context, size: 13),
                decoration: InputDecoration(
                  isDense: true,
                  prefixIcon: const Icon(Icons.search_rounded, size: 18),
                  hintText: _regex ? r'regular expression, e.g. (ERROR|WARN)' : 'Find text',
                ),
                onSubmitted: (_) => _search(),
              ),
            ),
            FilterChip(label: Text(s.observeRegex), selected: _regex, onSelected: (v) => setState(() => _regex = v)),
            DropdownButton<String>(
              value: _range,
              items: [
                DropdownMenuItem(value: '1h', child: Text(s.observeLastHour)),
                DropdownMenuItem(value: '6h', child: Text(s.observeLast6Hours)),
                DropdownMenuItem(value: '24h', child: Text(s.observeLast24Hours)),
                DropdownMenuItem(value: '7d', child: Text(s.observeLast7Days)),
                DropdownMenuItem(value: 'all', child: Text(s.observeEverythingKept)),
              ],
              onChanged: (v) {
                setState(() => _range = v ?? '24h');
                _search();
              },
            ),
            FilledButton(onPressed: _busy ? null : () => _search(), child: Text(s.observeSearch)),
            IconButton(
              tooltip: s.observeCopyShownLines,
              icon: const Icon(Icons.copy_rounded, size: 18),
              onPressed: _lines.isEmpty
                  ? null
                  : () {
                      Clipboard.setData(ClipboardData(text: _lines.map((l) => '${l.time.toIso8601String()} ${l.pod} ${l.line}').join('\n')));
                      showSnack(context, s.observeCopiedLines(_lines.length));
                    },
            ),
          ],
        ),
        const SizedBox(height: 8),
        if (_error != null) Padding(padding: const EdgeInsets.only(bottom: 8), child: Text(_error!, style: TextStyle(color: c.failed))),
        Container(
          height: 520,
          decoration: BoxDecoration(color: c.background, border: Border.all(color: c.border), borderRadius: BorderRadius.circular(NectarTokens.radius)),
          child: _lines.isEmpty
              ? Center(
                  child: _busy
                      ? const CircularProgressIndicator()
                      : Text(_searched ? 'Nothing found in this time range.' : '', style: TextStyle(color: c.muted)),
                )
              : ListView.builder(
                  padding: const EdgeInsets.all(10),
                  itemCount: _lines.length + (_more ? 1 : 0),
                  itemBuilder: (context, i) {
                    if (i == _lines.length) {
                      return Center(
                        child: TextButton(onPressed: _busy ? null : () => _search(more: true), child: Text(_busy ? 'Loading…' : 'Load older lines')),
                      );
                    }
                    final l = _lines[i];
                    return SelectableText.rich(
                      TextSpan(children: [
                        TextSpan(text: '${_stamp(l.time)}  ', style: TextStyle(color: c.muted)),
                        TextSpan(text: '${l.pod.length > 18 ? '…${l.pod.substring(l.pod.length - 17)}' : l.pod}  ', style: TextStyle(color: NectarTokens.amber.withValues(alpha: 0.8))),
                        TextSpan(text: l.line),
                      ]),
                      style: monoStyle(context, size: 12.5),
                    );
                  },
                ),
        ),
        const SizedBox(height: 6),
        Text(s.observeNewestFirstLogsAreKept, style: TextStyle(color: c.muted, fontSize: 12)),
      ],
    );
  }
}
