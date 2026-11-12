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

import 'dart:typed_data';
import 'dart:ui' as ui;

/// One size of the tray icon as StatusNotifierItem's IconPixmap wants it:
/// ARGB32, one pixel after another, each in network byte order (A, R, G, B).
class TrayPixmap {
  const TrayPixmap(this.width, this.height, this.argb);
  final int width;
  final int height;
  final Uint8List argb;
}

/// Sizes the tray host picks from; panels draw 16 to 32 px, HiDPI doubles.
const trayIconSizes = [16, 22, 24, 32, 48, 64];

const _green = ui.Color(0xFF00FF41);

/// Draws the tray glyph into a [size]×[size] square: the app's bubble with a
/// padlock, closed while the vault is locked and open while it is unlocked,
/// so the panel shows the vault state at a glance. Drawn on a 24-unit grid.
void paintTrayIcon(ui.Canvas canvas, double size, {required bool unlocked}) {
  canvas.save();
  canvas.scale(size / 24);
  final stroke = ui.Paint()
    ..color = _green
    ..style = ui.PaintingStyle.stroke
    ..strokeWidth = 1.6
    ..strokeCap = ui.StrokeCap.round
    ..isAntiAlias = true;
  final fill = ui.Paint()
    ..color = _green
    ..isAntiAlias = true;

  canvas.drawCircle(const ui.Offset(12, 12), 10.6, stroke);

  // Shackle: closed, both legs reach into the body. Open, the arc rises and
  // the left leg ends well above the body, while the right one stays in it.
  final legTop = unlocked ? 7.4 : 9.4;
  final shackle = ui.Path()
    ..moveTo(9.4, unlocked ? 8.4 : 11.5)
    ..lineTo(9.4, legTop)
    ..arcToPoint(ui.Offset(14.6, legTop),
        radius: const ui.Radius.circular(2.6))
    ..lineTo(14.6, 11.5);
  canvas.drawPath(shackle, stroke..strokeWidth = 1.7);

  // Body with the keyhole cut out.
  canvas.saveLayer(const ui.Rect.fromLTWH(0, 0, 24, 24), ui.Paint());
  canvas.drawRRect(
      ui.RRect.fromRectAndRadius(const ui.Rect.fromLTWH(7.4, 11, 9.2, 7.4),
          const ui.Radius.circular(1.4)),
      fill);
  final hole = ui.Paint()..blendMode = ui.BlendMode.clear;
  canvas.drawCircle(const ui.Offset(12, 14), 1.2, hole);
  canvas.drawRect(const ui.Rect.fromLTWH(11.45, 14, 1.1, 2.3), hole);
  canvas.restore();

  canvas.restore();
}

/// Converts straight (not premultiplied) RGBA bytes to IconPixmap's ARGB.
Uint8List rgbaToArgb(Uint8List rgba) {
  final out = Uint8List(rgba.length);
  for (var i = 0; i + 3 < rgba.length; i += 4) {
    out[i] = rgba[i + 3];
    out[i + 1] = rgba[i];
    out[i + 2] = rgba[i + 1];
    out[i + 3] = rgba[i + 2];
  }
  return out;
}

Future<TrayPixmap> renderTrayIcon(int size, {required bool unlocked}) async {
  final recorder = ui.PictureRecorder();
  paintTrayIcon(ui.Canvas(recorder), size.toDouble(), unlocked: unlocked);
  final picture = recorder.endRecording();
  final image = await picture.toImage(size, size);
  picture.dispose();
  final data =
      await image.toByteData(format: ui.ImageByteFormat.rawStraightRgba);
  image.dispose();
  return TrayPixmap(size, size, rgbaToArgb(data!.buffer.asUint8List()));
}

/// Every size in [trayIconSizes], for one vault state.
Future<List<TrayPixmap>> renderTrayIcons({required bool unlocked}) async => [
      for (final size in trayIconSizes)
        await renderTrayIcon(size, unlocked: unlocked),
    ];
