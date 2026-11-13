import 'package:material_ui/material_ui.dart';
import 'package:nectar_apps_ui/widgets/highlights/syntax_rule.dart';

final htmlSyntaxRules = <SyntaxRule>[
  // 1. HTML comments
  SyntaxRule(
    RegExp(r'<!--[\s\S]*?-->'),
    const TextStyle(color: Color(0xFF7F848E), fontStyle: FontStyle.italic),
  ),

  // 2. DOCTYPE
  SyntaxRule(
    RegExp(r'<!DOCTYPE\b[^>]*>'),
    const TextStyle(color: Color(0xFFC678DD), fontWeight: FontWeight.bold),
  ),

  // 3. CDATA
  SyntaxRule(
    RegExp(r'<!\[CDATA\[[\s\S]*?\]\]>'),
    const TextStyle(color: Color(0xFF98C379)),
  ),

  // 4. Closing and opening tag names
  SyntaxRule(
    RegExp(r'</?[a-zA-Z][a-zA-Z0-9:_-]*'),
    const TextStyle(color: Color(0xFFE06C75), fontWeight: FontWeight.bold),
  ),

  // 5. Self-closing tag ending
  SyntaxRule(RegExp(r'/?>'), const TextStyle(color: Color(0xFFE06C75))),

  // 6. Attribute names
  SyntaxRule(
    RegExp(r'\b[a-zA-Z_:][a-zA-Z0-9:._-]*(?=\s*=)'),
    const TextStyle(color: Color(0xFFD19A66)),
  ),

  // 7. Attribute strings (double and single quotes)
  SyntaxRule(
    RegExp(r'''"[^"]*"|'[^']*' '''),
    const TextStyle(color: Color(0xFF98C379)),
  ),

  // 8. Attribute values without quotes
  SyntaxRule(
    RegExp(r'(?<==)\s*[^\s>]+'),
    const TextStyle(color: Color(0xFF98C379)),
  ),

  // 9. HTML entities
  SyntaxRule(
    RegExp(r'&(?:[a-zA-Z][a-zA-Z0-9]+|#\d+|#x[0-9a-fA-F]+);'),
    const TextStyle(color: Color(0xFFE5C07B)),
  ),

  // 10. Template placeholders
  SyntaxRule(
    RegExp(r'\{\{[\s\S]*?\}\}'),
    const TextStyle(color: Color(0xFFC678DD)),
  ),

  // 11. HTML declarations
  SyntaxRule(RegExp(r'<![^>]+>'), const TextStyle(color: Color(0xFFC678DD))),

  // 12. Numeric values
  SyntaxRule(
    RegExp(r'\b\d+(?:\.\d+)?(?:px|em|rem|%|vh|vw)?\b'),
    const TextStyle(color: Color(0xFFD19A66)),
  ),
];
