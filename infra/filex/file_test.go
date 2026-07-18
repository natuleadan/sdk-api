package filex

import (
	"os"
	"testing"

	"github.com/natuleadan/sdk-api/infra/fs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	longLine      = `Quid securi etiam tamquam eu fugiat nulla pariatur. Nec dubitamus multa iter quae et nos invenerat. Non equidem invideo, miror magis posuere velit aliquet. Integer legentibus erat a ante historiarum dapibus. Prima luce, cum quibus mons aliud consensu ab eo.Quid securi etiam tamquam eu fugiat nulla pariatur. Nec dubitamus multa iter quae et nos invenerat. Non equidem invideo, miror magis posuere velit aliquet. Integer legentibus erat a ante historiarum dapibus. Prima luce, cum quibus mons aliud consensu ab eo.Quid securi etiam tamquam eu fugiat nulla pariatur. Nec dubitamus multa iter quae et nos invenerat. Non equidem invideo, miror magis posuere velit aliquet. Integer legentibus erat a ante historiarum dapibus. Prima luce, cum quibus mons aliud consensu ab eo.Quid securi etiam tamquam eu fugiat nulla pariatur. Nec dubitamus multa iter quae et nos invenerat. Non equidem invideo, miror magis posuere velit aliquet. Integer legentibus erat a ante historiarum dapibus. Prima luce, cum quibus mons aliud consensu ab eo.Quid securi etiam tamquam eu fugiat nulla pariatur. Nec dubitamus multa iter quae et nos invenerat. Non equidem invideo, miror magis posuere velit aliquet. Integer legentibus erat a ante historiarum dapibus. Prima luce, cum quibus mons aliud consensu ab eo.`
	longFirstLine = longLine + "\n" + text
	text          = `first line
Cum sociis natoque penatibus et magnis dis parturient. Phasellus laoreet lorem vel dolor tempus vehicula. Vivamus sagittis lacus vel augue laoreet rutrum faucibus. Integer legentibus erat a ante historiarum dapibus.
Quisque ut dolor gravida, placerat libero vel, euismod. Quam temere in vitiis, legem sancimus haerentia. Qui ipsorum lingua Celtae, nostra Galli appellantur. Quis aute iure reprehenderit in voluptate velit esse. Fabio vel iudice vincam, sunt in culpa qui officia. Cras mattis iudicium purus sit amet fermentum.
Quo usque tandem abutere, Catilina, patientia nostra? Gallia est omnis divisa in partes tres, quarum. Quam diu etiam furor iste tuus nos eludet? Quid securi etiam tamquam eu fugiat nulla pariatur. Curabitur blandit tempus ardua ridiculous sed magna.
Magna pars studiorum, prodita quaerimus. Cum ceteris in veneratione tui montes, nascetur mus. Morbi odio eros, volutpat ut pharetra vitae, lobortis sed nibh. Plura mihi bona sunt, inclinet, amari petere vellent. Idque Caesaris facere voluntate liceret: sese habere. Tu quoque, Brute, fili mi, nihil timor populi, nihil!
Tityre, tu patulae recubans sub tegmine fagi dolor. Inmensae subtilitatis, obscuris et malesuada fames. Quae vero auctorem tractata ab fiducia dicuntur.
Cum sociis natoque penatibus et magnis dis parturient. Phasellus laoreet lorem vel dolor tempus vehicula. Vivamus sagittis lacus vel augue laoreet rutrum faucibus. Integer legentibus erat a ante historiarum dapibus.
Quisque ut dolor gravida, placerat libero vel, euismod. Quam temere in vitiis, legem sancimus haerentia. Qui ipsorum lingua Celtae, nostra Galli appellantur. Quis aute iure reprehenderit in voluptate velit esse. Fabio vel iudice vincam, sunt in culpa qui officia. Cras mattis iudicium purus sit amet fermentum.
Quo usque tandem abutere, Catilina, patientia nostra? Gallia est omnis divisa in partes tres, quarum. Quam diu etiam furor iste tuus nos eludet? Quid securi etiam tamquam eu fugiat nulla pariatur. Curabitur blandit tempus ardua ridiculous sed magna.
Magna pars studiorum, prodita quaerimus. Cum ceteris in veneratione tui montes, nascetur mus. Morbi odio eros, volutpat ut pharetra vitae, lobortis sed nibh. Plura mihi bona sunt, inclinet, amari petere vellent. Idque Caesaris facere voluntate liceret: sese habere. Tu quoque, Brute, fili mi, nihil timor populi, nihil!
Tityre, tu patulae recubans sub tegmine fagi dolor. Inmensae subtilitatis, obscuris et malesuada fames. Quae vero auctorem tractata ab fiducia dicuntur.
Cum sociis natoque penatibus et magnis dis parturient. Phasellus laoreet lorem vel dolor tempus vehicula. Vivamus sagittis lacus vel augue laoreet rutrum faucibus. Integer legentibus erat a ante historiarum dapibus.
Quisque ut dolor gravida, placerat libero vel, euismod. Quam temere in vitiis, legem sancimus haerentia. Qui ipsorum lingua Celtae, nostra Galli appellantur. Quis aute iure reprehenderit in voluptate velit esse. Fabio vel iudice vincam, sunt in culpa qui officia. Cras mattis iudicium purus sit amet fermentum.
Quo usque tandem abutere, Catilina, patientia nostra? Gallia est omnis divisa in partes tres, quarum. Quam diu etiam furor iste tuus nos eludet? Quid securi etiam tamquam eu fugiat nulla pariatur. Curabitur blandit tempus ardua ridiculous sed magna.
Magna pars studiorum, prodita quaerimus. Cum ceteris in veneratione tui montes, nascetur mus. Morbi odio eros, volutpat ut pharetra vitae, lobortis sed nibh. Plura mihi bona sunt, inclinet, amari petere vellent. Idque Caesaris facere voluntate liceret: sese habere. Tu quoque, Brute, fili mi, nihil timor populi, nihil!
Tityre, tu patulae recubans sub tegmine fagi dolor. Inmensae subtilitatis, obscuris et malesuada fames. Quae vero auctorem tractata ab fiducia dicuntur.
` + longLine
	textWithLastNewline = `first line
Cum sociis natoque penatibus et magnis dis parturient. Phasellus laoreet lorem vel dolor tempus vehicula. Vivamus sagittis lacus vel augue laoreet rutrum faucibus. Integer legentibus erat a ante historiarum dapibus.
Quisque ut dolor gravida, placerat libero vel, euismod. Quam temere in vitiis, legem sancimus haerentia. Qui ipsorum lingua Celtae, nostra Galli appellantur. Quis aute iure reprehenderit in voluptate velit esse. Fabio vel iudice vincam, sunt in culpa qui officia. Cras mattis iudicium purus sit amet fermentum.
Quo usque tandem abutere, Catilina, patientia nostra? Gallia est omnis divisa in partes tres, quarum. Quam diu etiam furor iste tuus nos eludet? Quid securi etiam tamquam eu fugiat nulla pariatur. Curabitur blandit tempus ardua ridiculous sed magna.
Magna pars studiorum, prodita quaerimus. Cum ceteris in veneratione tui montes, nascetur mus. Morbi odio eros, volutpat ut pharetra vitae, lobortis sed nibh. Plura mihi bona sunt, inclinet, amari petere vellent. Idque Caesaris facere voluntate liceret: sese habere. Tu quoque, Brute, fili mi, nihil timor populi, nihil!
Tityre, tu patulae recubans sub tegmine fagi dolor. Inmensae subtilitatis, obscuris et malesuada fames. Quae vero auctorem tractata ab fiducia dicuntur.
Cum sociis natoque penatibus et magnis dis parturient. Phasellus laoreet lorem vel dolor tempus vehicula. Vivamus sagittis lacus vel augue laoreet rutrum faucibus. Integer legentibus erat a ante historiarum dapibus.
Quisque ut dolor gravida, placerat libero vel, euismod. Quam temere in vitiis, legem sancimus haerentia. Qui ipsorum lingua Celtae, nostra Galli appellantur. Quis aute iure reprehenderit in voluptate velit esse. Fabio vel iudice vincam, sunt in culpa qui officia. Cras mattis iudicium purus sit amet fermentum.
Quo usque tandem abutere, Catilina, patientia nostra? Gallia est omnis divisa in partes tres, quarum. Quam diu etiam furor iste tuus nos eludet? Quid securi etiam tamquam eu fugiat nulla pariatur. Curabitur blandit tempus ardua ridiculous sed magna.
Magna pars studiorum, prodita quaerimus. Cum ceteris in veneratione tui montes, nascetur mus. Morbi odio eros, volutpat ut pharetra vitae, lobortis sed nibh. Plura mihi bona sunt, inclinet, amari petere vellent. Idque Caesaris facere voluntate liceret: sese habere. Tu quoque, Brute, fili mi, nihil timor populi, nihil!
Tityre, tu patulae recubans sub tegmine fagi dolor. Inmensae subtilitatis, obscuris et malesuada fames. Quae vero auctorem tractata ab fiducia dicuntur.
Cum sociis natoque penatibus et magnis dis parturient. Phasellus laoreet lorem vel dolor tempus vehicula. Vivamus sagittis lacus vel augue laoreet rutrum faucibus. Integer legentibus erat a ante historiarum dapibus.
Quisque ut dolor gravida, placerat libero vel, euismod. Quam temere in vitiis, legem sancimus haerentia. Qui ipsorum lingua Celtae, nostra Galli appellantur. Quis aute iure reprehenderit in voluptate velit esse. Fabio vel iudice vincam, sunt in culpa qui officia. Cras mattis iudicium purus sit amet fermentum.
Quo usque tandem abutere, Catilina, patientia nostra? Gallia est omnis divisa in partes tres, quarum. Quam diu etiam furor iste tuus nos eludet? Quid securi etiam tamquam eu fugiat nulla pariatur. Curabitur blandit tempus ardua ridiculous sed magna.
Magna pars studiorum, prodita quaerimus. Cum ceteris in veneratione tui montes, nascetur mus. Morbi odio eros, volutpat ut pharetra vitae, lobortis sed nibh. Plura mihi bona sunt, inclinet, amari petere vellent. Idque Caesaris facere voluntate liceret: sese habere. Tu quoque, Brute, fili mi, nihil timor populi, nihil!
Tityre, tu patulae recubans sub tegmine fagi dolor. Inmensae subtilitatis, obscuris et malesuada fames. Quae vero auctorem tractata ab fiducia dicuntur.
` + longLine + "\n"
	shortText = `first line
second line
last line`
	shortTextWithLastNewline = `first line
second line
last line
`
	emptyContent = ``
)

func TestFirstLine(t *testing.T) {
	filename, err := fs.TempFilenameWithText(longFirstLine)
	require.NoError(t, err)
	defer os.Remove(filename)

	val, err := FirstLine(filename)
	require.NoError(t, err)
	assert.Equal(t, longLine, val)
}

func TestFirstLineShort(t *testing.T) {
	filename, err := fs.TempFilenameWithText(shortText)
	require.NoError(t, err)
	defer os.Remove(filename)

	val, err := FirstLine(filename)
	require.NoError(t, err)
	assert.Equal(t, "first line", val)
}

func TestFirstLineError(t *testing.T) {
	_, err := FirstLine("/tmp/does-not-exist")
	require.Error(t, err)
}

func TestFirstLineEmptyFile(t *testing.T) {
	filename, err := fs.TempFilenameWithText(emptyContent)
	require.NoError(t, err)
	defer os.Remove(filename)

	val, err := FirstLine(filename)
	require.NoError(t, err)
	assert.Empty(t, val)
}

func TestFirstLineWithoutNewline(t *testing.T) {
	filename, err := fs.TempFilenameWithText(longLine)
	require.NoError(t, err)
	defer os.Remove(filename)

	val, err := FirstLine(filename)
	require.NoError(t, err)
	assert.Equal(t, longLine, val)
}

func TestLastLine(t *testing.T) {
	filename, err := fs.TempFilenameWithText(text)
	require.NoError(t, err)
	defer os.Remove(filename)

	val, err := LastLine(filename)
	require.NoError(t, err)
	assert.Equal(t, longLine, val)
}

func TestLastLineWithLastNewline(t *testing.T) {
	filename, err := fs.TempFilenameWithText(textWithLastNewline)
	require.NoError(t, err)
	defer os.Remove(filename)

	val, err := LastLine(filename)
	require.NoError(t, err)
	assert.Equal(t, longLine, val)
}

func TestLastLineWithoutLastNewline(t *testing.T) {
	filename, err := fs.TempFilenameWithText(longLine)
	require.NoError(t, err)
	defer os.Remove(filename)

	val, err := LastLine(filename)
	require.NoError(t, err)
	assert.Equal(t, longLine, val)
}

func TestLastLineShort(t *testing.T) {
	filename, err := fs.TempFilenameWithText(shortText)
	require.NoError(t, err)
	defer os.Remove(filename)

	val, err := LastLine(filename)
	require.NoError(t, err)
	assert.Equal(t, "last line", val)
}

func TestLastLineWithLastNewlineShort(t *testing.T) {
	filename, err := fs.TempFilenameWithText(shortTextWithLastNewline)
	require.NoError(t, err)
	defer os.Remove(filename)

	val, err := LastLine(filename)
	require.NoError(t, err)
	assert.Equal(t, "last line", val)
}

func TestLastLineError(t *testing.T) {
	_, err := LastLine("/tmp/does-not-exist")
	require.Error(t, err)
}

func TestLastLineEmptyFile(t *testing.T) {
	filename, err := fs.TempFilenameWithText(emptyContent)
	require.NoError(t, err)
	defer os.Remove(filename)

	val, err := LastLine(filename)
	require.NoError(t, err)
	assert.Empty(t, val)
}

func TestFirstLineExactlyBufSize(t *testing.T) {
	content := make([]byte, bufSize)
	for i := range content {
		content[i] = 'a'
	}
	content[bufSize-1] = '\n' // Ensure there is a newline at the edge

	filename, err := fs.TempFilenameWithText(string(content))
	require.NoError(t, err)
	defer os.Remove(filename)

	val, err := FirstLine(filename)
	require.NoError(t, err)
	assert.Equal(t, string(content[:bufSize-1]), val)
}

func TestLastLineExactlyBufSize(t *testing.T) {
	content := make([]byte, bufSize)
	for i := range content {
		content[i] = 'a'
	}
	content[bufSize-1] = '\n' // Ensure there is a newline at the edge

	filename, err := fs.TempFilenameWithText(string(content))
	require.NoError(t, err)
	defer os.Remove(filename)

	val, err := LastLine(filename)
	require.NoError(t, err)
	assert.Equal(t, string(content[:bufSize-1]), val)
}

func TestFirstLineLargeFile(t *testing.T) {
	content := text + text + text + "\n" + "extra"
	filename, err := fs.TempFilenameWithText(content)
	require.NoError(t, err)
	defer os.Remove(filename)

	val, err := FirstLine(filename)
	require.NoError(t, err)
	assert.Equal(t, "first line", val)
}

func TestLastLineLargeFile(t *testing.T) {
	content := text + text + text + "\n" + "extra"
	filename, err := fs.TempFilenameWithText(content)
	require.NoError(t, err)
	defer os.Remove(filename)

	val, err := LastLine(filename)
	require.NoError(t, err)
	assert.Equal(t, "extra", val)
}
