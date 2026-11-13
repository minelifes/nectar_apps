import 'package:material_ui/material_ui.dart';
import 'package:nectar_apps_ui/widgets/highlights/syntax_rule.dart';

final jsonSyntaxRules = <SyntaxRule>[
  // Strings used as object keys
  SyntaxRule(
    RegExp(r'"(?:[^"\\]|\\.)*"(?=\s*:)'),
    const TextStyle(color: Color(0xFF56B6C2)),
  ),

  // String values
  SyntaxRule(
    RegExp(r'"(?:[^"\\]|\\.)*"'),
    const TextStyle(color: Color(0xFF98C379)),
  ),

  // Numbers
  SyntaxRule(
    RegExp(r'-?(?:0|[1-9]\d*)(?:\.\d+)?(?:[eE][+-]?\d+)?'),
    const TextStyle(color: Color(0xFFD19A66)),
  ),

  // Booleans
  SyntaxRule(
    RegExp(r'\b(?:true|false)\b'),
    const TextStyle(color: Color(0xFFC678DD)),
  ),

  // Null
  SyntaxRule(RegExp(r'\bnull\b'), const TextStyle(color: Color(0xFFE06C75))),

  // Structural punctuation
  SyntaxRule(RegExp(r'[{}\[\],:]'), const TextStyle(color: Color(0xFFABB2BF))),
];
