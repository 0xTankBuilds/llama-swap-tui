import os, re, sys
from PIL import Image, ImageDraw, ImageFont

FONT_R="/usr/share/fonts/truetype/dejavu/DejaVuSansMono.ttf"
FONT_B="/usr/share/fonts/truetype/dejavu/DejaVuSansMono-Bold.ttf"
SIZE=17

def build_palette():
    pal={}
    base=[(0,0,0),(128,0,0),(0,128,0),(128,128,0),(0,0,128),(128,0,128),(0,128,128),(192,192,192),
          (128,128,128),(255,0,0),(0,255,0),(255,255,0),(0,0,255),(255,0,255),(0,255,255),(255,255,255)]
    for i,c in enumerate(base): pal[i]=c
    c6={0:0,1:85,2:170,3:255,4:255,5:255}
    for i in range(16,232):
        r,g,b=(i-16)//36,(i-16)%36//6,(i-16)%6
        pal[i]=(c6[r],c6[g],c6[b])
    for i in range(232,256):
        v=10+(i-232)*10
        pal[i]=(v,v,v)
    return pal
PAL=build_palette()
TERM_BG=(22,22,22)
DEFAULT_FG=(222,222,222)

def rfg(c):
    if c is None: return DEFAULT_FG
    if isinstance(c,tuple): return (c[1],c[2],c[3])
    return PAL.get(c,DEFAULT_FG)
def rbg(c):
    if c is None: return TERM_BG
    if isinstance(c,tuple): return (c[1],c[2],c[3])
    return PAL.get(c,TERM_BG)

pat=re.compile(rb'\x1b\[([0-9;]*)m')
def parse_line(raw):
    cols=[]; fg=bg=None; bold=False; pos=0
    for m in pat.finditer(raw):
        text=raw[pos:m.start()].decode('utf-8','replace')
        for ch in text: cols.append((ch,fg,bg,bold))
        nums=[int(x) for x in m.group(1).decode().split(';')] if m.group(1) else [0]
        j=0
        while j<len(nums):
            n=nums[j]
            if n==0: fg=bg=None; bold=False
            elif n==1: bold=True
            elif n in (22,24): bold=False
            elif n==39: fg=None
            elif n==49: bg=None
            elif 30<=n<=37: fg=n-30
            elif 90<=n<=97: fg=n-90+8
            elif 40<=n<=47: bg=n-40
            elif 100<=n<=107: bg=n-100+8
            elif n==38:
                if j+2<len(nums) and nums[j+1]==5: fg=nums[j+2]; j+=2
                elif j+3<len(nums) and nums[j+1]==2: fg=('#rgb',nums[j+2],nums[j+3],nums[j+4]); j+=3
            elif n==48:
                if j+2<len(nums) and nums[j+1]==5: bg=nums[j+2]; j+=2
                elif j+3<len(nums) and nums[j+1]==2: bg=('#rgb',nums[j+2],nums[j+3],nums[j+4]); j+=3
            j+=1
        pos=m.end()
    text=raw[pos:].decode('utf-8','replace')
    for ch in text: cols.append((ch,fg,bg,bold))
    return cols

TERMW=128
def render(ansi, png):
    lines=open(ansi,'rb').read().split(b'\n')
    parsed=[parse_line(l) for l in lines]
    while parsed and all(c[0] in ('',' ') for c in parsed[-1]): parsed.pop()
    font=ImageFont.truetype(FONT_R,SIZE); fontb=ImageFont.truetype(FONT_B,SIZE)
    asc,desc=font.getmetrics()
    cw=round(font.getlength("M"))
    ch=asc+desc+5
    W=TERMW*cw; H=len(parsed)*ch
    img=Image.new("RGB",(W,H),TERM_BG); d=ImageDraw.Draw(img)
    ypad=(ch-(asc+desc))//2
    for ri,cols in enumerate(parsed):
        base_y=ri*ch
        for ci in range(TERMW):
            if ci<len(cols): char,fg,bg,bold=cols[ci]
            else: char,fg,bg,bold=(' ',None,None,False)
            x=ci*cw
            d.rectangle([x,base_y,x+cw,base_y+ch],fill=rbg(bg))
            d.text((x,base_y+ypad),char,fill=rfg(fg),font=fontb if bold else font)
    img.save(png)
    return W,H,len(parsed)

# usage: mkshot.py <out_dir> [<name,name,...>] [<in_dir>]
out=sys.argv[1]
os.makedirs(out, exist_ok=True)
names=sys.argv[2].split(',') if len(sys.argv)>2 else ["status","activity","models","hardware","logs"]
in_dir=sys.argv[3] if len(sys.argv)>3 else "/tmp/screenshots"
for name in names:
    w,h,rows=render(f"{in_dir}/{name}.ansi", f"{out}/{name}.png")
    print(f"{name}.png {w}x{h} ({rows} rows)")
