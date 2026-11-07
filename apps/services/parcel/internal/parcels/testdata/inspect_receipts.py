import sys
from pathlib import Path

import pymupdf as fitz
from pypdf import PdfReader
from PIL import Image,ImageDraw

root=Path(sys.argv[1]) if len(sys.argv)>1 else Path('tmp/m5/receipt-artifacts')
fontdir=Path('apps/services/parcel/internal/parcels/receiptfonts').resolve()
cases={'latin':'José François Müller','latin_decomposed':'Jose\u0301 Franc\u0327ois','sinhala':'නිමල් පෙරේරා','tamil':'அமலி பெர்னாண்டோ','arabic':'محمد أحمد','arabic_marks':'مُحَمَّد أَحْمَد','hebrew':'דוד לוי','hebrew_marks':'דָּוִד לֵוִי'}
canvas=Image.new('RGB',(1160,len(cases)*185),'white');draw=ImageDraw.Draw(canvas)
fonts=['NotoSans','NotoSansSinhala','NotoSansTamil','NotoSansArabic','NotoSansHebrew']
css=''.join('@font-face{font-family:'+f+';src:url('+f+'-Regular.ttf)}' for f in fonts)+'body{margin:0;font-size:12pt;line-height:22pt;font-family:'+','.join(fonts)+';}'
for index,(name,value) in enumerate(cases.items()):
    path=root/(name+'.pdf')
    parsed=PdfReader(str(path),strict=True)
    extracted='\n'.join(p.extract_text() or '' for p in parsed.pages)
    assert value in extracted,(name,repr(extracted))
    assert 'Total fee: 550.00' in extracted
    doc=fitz.open(str(path));p=doc[0]
    assert value in p.get_text(), (name, repr(p.get_text()))
    pix=p.get_pixmap(matrix=fitz.Matrix(2,2),alpha=False)
    image=Image.frombytes('RGB',[pix.width,pix.height],pix.samples)
    image.save(root/(name+'.png'))
    actual=image.crop((80,158,1120,218))
    native=fitz.open();reference=native.new_page(width=595,height=842)
    family={'latin':'NotoSans','sinhala':'NotoSansSinhala','tamil':'NotoSansTamil','arabic':'NotoSansArabic','hebrew':'NotoSansHebrew'}[name.split('_')[0]]
    reference.insert_htmlbox(fitz.Rect(48,80,550,135),'Sender: <span style="font-family:'+family+'">'+value+'</span>',css=css,archive=fitz.Archive(str(fontdir)))
    npix=reference.get_pixmap(matrix=fitz.Matrix(2,2),alpha=False)
    nimage=Image.frombytes('RGB',[npix.width,npix.height],npix.samples)
    reference_image=nimage.crop((80,158,1120,230))
    y=index*185
    draw.text((10,y+3),name+' - actual PDF outlines',fill='black')
    canvas.paste(actual,(80,y+22))
    draw.text((10,y+91),'native font reference',fill='black')
    canvas.paste(reference_image,(80,y+110))
    print(name+': PASS strict PDF parse; exact Unicode text; original 550.00 fee; rasterized '+str(pix.width)+'x'+str(pix.height))
canvas.save(root/'comparison.png')
print('Comparison: '+str(root/'comparison.png'))
