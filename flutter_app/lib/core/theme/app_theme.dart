// Copyright (C) 2026 Gerald Hofbauer <info@geraldhofbauer.net>
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as
// published by the Free Software Foundation, either version 3 of the
// License, or (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

import 'package:flutter/material.dart';
import 'package:google_fonts/google_fonts.dart';

/// Passbubble design system: angular/square, terminal green, #212121 bg, blur effects.
class AppTheme {
  static const Color bg = Color(0xFF212121);
  static const Color surface = Color(0xFF2A2A2A);
  static const Color surfaceVariant = Color(0xFF303030);
  static const Color green = Color(0xFF00E676);
  static const Color greenDim = Color(0xFF00C853);
  static const Color greenFaint = Color(0xFF1B2A1F);
  static const Color onBg = Color(0xFFE0E0E0);
  static const Color onBgDim = Color(0xFF9E9E9E);
  static const Color error = Color(0xFFCF6679);
  static const Color border = Color(0xFF424242);

  static ThemeData get dark {
    final base = ThemeData.dark(useMaterial3: true);
    return base.copyWith(
      scaffoldBackgroundColor: bg,
      colorScheme: const ColorScheme.dark(
        brightness: Brightness.dark,
        primary: green,
        onPrimary: bg,
        secondary: greenDim,
        onSecondary: bg,
        surface: surface,
        onSurface: onBg,
        surfaceContainerHighest: surfaceVariant,
        error: error,
        outline: border,
      ),
      textTheme: GoogleFonts.jetBrainsMonoTextTheme(base.textTheme).apply(
        bodyColor: onBg,
        displayColor: green,
      ),
      appBarTheme: AppBarTheme(
        backgroundColor: bg,
        foregroundColor: green,
        elevation: 0,
        centerTitle: false,
        titleTextStyle: GoogleFonts.jetBrainsMono(
          fontSize: 16,
          fontWeight: FontWeight.w600,
          color: green,
          letterSpacing: 1.2,
        ),
      ),
      cardTheme: CardThemeData(
        color: surface,
        elevation: 0,
        shape: const RoundedRectangleBorder(
          borderRadius: BorderRadius.zero,
          side: BorderSide(color: border),
        ),
      ),
      inputDecorationTheme: InputDecorationTheme(
        filled: true,
        fillColor: surfaceVariant,
        border: const OutlineInputBorder(
          borderRadius: BorderRadius.zero,
          borderSide: BorderSide(color: border),
        ),
        enabledBorder: const OutlineInputBorder(
          borderRadius: BorderRadius.zero,
          borderSide: BorderSide(color: border),
        ),
        focusedBorder: const OutlineInputBorder(
          borderRadius: BorderRadius.zero,
          borderSide: BorderSide(color: green, width: 1.5),
        ),
        labelStyle: const TextStyle(color: onBgDim),
        hintStyle: const TextStyle(color: onBgDim),
        prefixIconColor: onBgDim,
      ),
      elevatedButtonTheme: ElevatedButtonThemeData(
        style: ElevatedButton.styleFrom(
          backgroundColor: green,
          foregroundColor: bg,
          shape: const RoundedRectangleBorder(borderRadius: BorderRadius.zero),
          textStyle: GoogleFonts.jetBrainsMono(
            fontWeight: FontWeight.w600,
            letterSpacing: 1.0,
          ),
        ),
      ),
      textButtonTheme: TextButtonThemeData(
        style: TextButton.styleFrom(
          foregroundColor: green,
          shape: const RoundedRectangleBorder(borderRadius: BorderRadius.zero),
        ),
      ),
      outlinedButtonTheme: OutlinedButtonThemeData(
        style: OutlinedButton.styleFrom(
          foregroundColor: green,
          side: const BorderSide(color: green),
          shape: const RoundedRectangleBorder(borderRadius: BorderRadius.zero),
        ),
      ),
      listTileTheme: const ListTileThemeData(
        tileColor: surface,
        textColor: onBg,
        iconColor: onBgDim,
        shape: Border(bottom: BorderSide(color: border)),
      ),
      dividerTheme: const DividerThemeData(color: border, thickness: 1),
      floatingActionButtonTheme: const FloatingActionButtonThemeData(
        backgroundColor: green,
        foregroundColor: bg,
        shape: RoundedRectangleBorder(borderRadius: BorderRadius.zero),
      ),
      chipTheme: ChipThemeData(
        backgroundColor: surfaceVariant,
        labelStyle: const TextStyle(color: onBg),
        side: const BorderSide(color: border),
        shape: const RoundedRectangleBorder(borderRadius: BorderRadius.zero),
      ),
      snackBarTheme: SnackBarThemeData(
        backgroundColor: surfaceVariant,
        contentTextStyle: GoogleFonts.jetBrainsMono(color: onBg),
        shape: const RoundedRectangleBorder(borderRadius: BorderRadius.zero),
        behavior: SnackBarBehavior.floating,
      ),
      dialogTheme: const DialogThemeData(
        backgroundColor: surface,
        shape: RoundedRectangleBorder(borderRadius: BorderRadius.zero),
      ),
      bottomSheetTheme: const BottomSheetThemeData(
        backgroundColor: surface,
        shape: RoundedRectangleBorder(borderRadius: BorderRadius.zero),
      ),
      navigationRailTheme: const NavigationRailThemeData(
        backgroundColor: bg,
        selectedIconTheme: IconThemeData(color: green),
        unselectedIconTheme: IconThemeData(color: onBgDim),
        selectedLabelTextStyle: TextStyle(color: green),
        unselectedLabelTextStyle: TextStyle(color: onBgDim),
        indicatorColor: greenFaint,
        useIndicator: true,
      ),
    );
  }
}
