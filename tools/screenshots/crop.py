# Crops the 2054x1444 captures to the README's 2000x1390, keeping the shadow
# around the window (85 px at the sides, 56 above, 114 below).
#
# usage: python3 crop.py RAW_DIR OUT_DIR
import glob
import os
import sys

from PIL import Image

src, out = sys.argv[1], sys.argv[2]
os.makedirs(out, exist_ok=True)
for f in sorted(glob.glob(os.path.join(src, '*.png'))):
    im = Image.open(f)
    assert im.size == (2054, 1444), (f, im.size)
    im.crop((27, 20, 2027, 1410)).save(os.path.join(out, os.path.basename(f)))
